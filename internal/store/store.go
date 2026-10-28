// Package store 是思港的文件存储层：vault 中的每个对象都是一个
// 「YAML frontmatter + Markdown 正文」文件。所有写操作经同一把
// flock 串行化（ADR-0003）。
//
// 设计哲学（v0.2）：目标导向而非进度导向——没有截止日期、没有紧急度，
// 只有重要程度（importance）与所服务的目标（goal）。
package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofrs/flock"
	"gopkg.in/yaml.v3"
)

// Meta 是 frontmatter 契约区。agent 只读写这里，不碰正文。
type Meta struct {
	Type        string     `yaml:"type" json:"type"` // thought | task | note
	ID          string     `yaml:"id,omitempty" json:"id,omitempty"`
	Title       string     `yaml:"title" json:"title"`
	Status      string     `yaml:"status,omitempty" json:"status,omitempty"`
	Importance  string     `yaml:"importance,omitempty" json:"importance,omitempty"` // critical|high|normal|someday
	Goal        string     `yaml:"goal,omitempty" json:"goal,omitempty"`
	Assignee    *string    `yaml:"assignee,omitempty" json:"assignee,omitempty"`
	AssignCwd   string     `yaml:"assign_cwd,omitempty" json:"assign_cwd,omitempty"` // 指定船位：只有该项目目录的会话能接令；空=任意会话
	// 挂船队：一个想法（任务）挂到一支具体船队（Claude 会话）上，与派船状态机正交。
	Session    string `yaml:"session,omitempty" json:"session,omitempty"`
	SessionDir string `yaml:"session_dir,omitempty" json:"session_dir,omitempty"`
	Tags        []string   `yaml:"tags,omitempty" json:"tags,omitempty"`
	Created     time.Time  `yaml:"created" json:"created"`
	Updated     *time.Time `yaml:"updated,omitempty" json:"updated,omitempty"`
	ClaimedAt   *time.Time `yaml:"claimed_at,omitempty" json:"claimed_at,omitempty"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty" json:"completed_at,omitempty"`
	BlockedAt   *time.Time `yaml:"blocked_at,omitempty" json:"blocked_at,omitempty"`
	Source      string     `yaml:"source,omitempty" json:"source,omitempty"`

	// Priority 是 v0.1 旧字段，读取时迁移为 importance，不再写出。
	Priority string `yaml:"priority,omitempty" json:"-"`
}

// AssigneeValue 给模板/JSON 展示用：未归属返回空串。
func (m Meta) AssigneeValue() string {
	if m.Assignee == nil {
		return ""
	}
	return *m.Assignee
}

// Record 是一个从磁盘读出（或待写入）的对象。
type Record struct {
	Meta Meta
	Body string
	Path string
}

// 任务状态（ADR-0003；v0.3 draft；v0.4 调度模型）。
// 人在网页端把想法立成集装箱：任务先以 draft 打磨，可停在 anchored 锚地
// 等待派船；派船（ready，指名 assignee）后只有被指派的船能接令。
const (
	StatusDraft      = "draft"       // 备货中：打磨描述与验收标准
	StatusAnchored   = "anchored"    // 锚地：已就绪但未派船，船看不到
	StatusReady      = "ready"       // 已派船待接令：assignee 必填且唯一可接令
	StatusInProgress = "in_progress" // 出港中
	StatusBlocked    = "blocked"     // 受阻上浮
	StatusDone       = "done"        // 归港（必须带结果，强制归港需理由）
	StatusCancelled  = "cancelled"
)

// 重要程度：目标导向，不谈紧急。
const (
	ImpCritical = "critical" // 关键——对目标起决定作用
	ImpHigh     = "high"     // 重要
	ImpNormal   = "normal"   //普通
	ImpSomeday  = "someday"  // 也许——漂流瓶里的事
)

var (
	// ErrNotFound 表示按编号找不到任务。
	ErrNotFound = errors.New("not found")
	// ErrInvalidTransition 表示状态跳转不合法。
	ErrInvalidTransition = errors.New("invalid status transition")
	// ErrInvalidImportance 表示重要程度取值非法。
	ErrInvalidImportance = errors.New("invalid importance")
	// ErrNotAssignee 表示任务未指派给该执行者，无权接令/推进。
	ErrNotAssignee = errors.New("任务未指派给你")
	// ErrWrongBerth 表示任务指定了船位（项目会话目录），当前目录的会话无权接令。
	ErrWrongBerth = errors.New("这票货派给了别的船位")
	// ErrNotDraft 表示只有未出港的任务（草稿/锚地）能删除，已派船的不能直接抹。
	ErrNotDraft = errors.New("只有未出港的任务（草稿/锚地）能删除")
	// ErrResultRequired 表示归港缺少交付结果（除非带理由强制归港）。
	ErrResultRequired = errors.New("归港必须交付结果")
)

// importanceRank 越小越重要，看板与 next 按此排序。
var importanceRank = map[string]int{
	ImpCritical: 0,
	ImpHigh:     1,
	ImpNormal:   2,
	ImpSomeday:  3,
}

// ValidImportance 判断取值是否合法（空串也合法，取默认 normal）。
func ValidImportance(s string) bool {
	if s == "" {
		return true
	}
	_, ok := importanceRank[s]
	return ok
}

// Vault 指向一个思港目录。
type Vault struct {
	Root string
	lock *flock.Flock
}

// Open 打开（必要时初始化）vault 的标准目录结构。
func Open(root string) (*Vault, error) {
	for _, d := range []string{
		"tasks", "notes", "archive", "attachments",
		".port/locks", ".port/state",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return nil, err
		}
	}
	return &Vault{
		Root: root,
		lock: flock.New(filepath.Join(root, ".port", "locks", "vault.lock")),
	}, nil
}

func (v *Vault) withLock(fn func() error) error {
	if err := v.lock.Lock(); err != nil {
		return fmt.Errorf("获取 vault 锁失败: %w", err)
	}
	defer func() { _ = v.lock.Unlock() }()
	return fn()
}

func touch(m *Meta) {
	now := time.Now()
	m.Updated = &now
}

// ---------- 任务 ----------

// NewTask 创建任务并分配 T 编号。draft=true 时为草稿（打磨中，不可领取、
// 不进 next），发布走 Publish；draft=false 直接进待领取泊位（CLI 常用）。
func (v *Vault) NewTask(title, importance, goal string, draft bool) (*Record, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("任务标题不能为空")
	}
	if !ValidImportance(importance) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidImportance, importance)
	}
	var rec *Record
	err := v.withLock(func() error {
		now := time.Now()
		id, err := v.nextID()
		if err != nil {
			return err
		}
		imp := importance
		if imp == "" {
			imp = ImpNormal
		}
		status := StatusAnchored
		createdNote := "created by port（锚地待派）"
		if draft {
			status = StatusDraft
			createdNote = "created as draft by port"
		}
		m := Meta{
			Type: "task", ID: id, Title: title,
			Status: status, Importance: imp, Goal: strings.TrimSpace(goal),
			Created: now,
		}
		body := "## 描述\n\n" + title + "\n\n## 验收标准\n\n## 日志\n" +
			"- " + now.Format("2006-01-02 15:04 -0700") + " — " + createdNote + "\n"
		path, err := v.uniquePath(filepath.Join(v.Root, "tasks", id+"-"+slug(title)+".md"))
		if err != nil {
			return err
		}
		if err := v.write(path, m, body); err != nil {
			return err
		}
		rec = &Record{Meta: m, Body: body, Path: path}
		return nil
	})
	return rec, err
}

// Claim 接令：ready → in_progress。只有被指派的船、且在指定船位
// （项目目录会话）里才能接令；接令即把任务号写到该会话心跳上。
func (v *Vault) Claim(id, by, cwd string) (*Record, error) {
	if r, err := v.GetTask(id); err == nil {
		if err := requireBerth(r, cwd); err != nil {
			return r, err
		}
	}
	rec, err := v.transition(id, "claim", "", by)
	if err != nil {
		return rec, err
	}
	_ = v.SetActorSessionTask(by, id) // 心跳失败不影响接令本身
	return rec, nil
}

// Done 归港：in_progress/blocked → done。
// 归港必须带结果——有验收项时须全部勾选；没有验收项时 result 必填。
// 死锁逃生门：force=true + 必填 reason 可强制归港，理由永久留痕。
// 归港后清掉心跳上的任务号。
func (v *Vault) Done(id, by, result string, force bool, reason string) (*Record, error) {
	result = strings.TrimSpace(result)
	reason = strings.TrimSpace(reason)
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		if r.Meta.Status != StatusInProgress && r.Meta.Status != StatusBlocked {
			return fmt.Errorf("%w: %s 当前状态为 %s，只有进行中/受阻的任务可归港", ErrInvalidTransition, id, r.Meta.Status)
		}
		if err := requireAssignee(r, by); err != nil {
			return err
		}
		doneN, total := acceptanceCount(r.Body)

		if force {
			if reason == "" {
				return errors.New("强制归港必须填写理由（留痕）")
			}
		} else {
			switch {
			case total > 0 && doneN < total:
				return fmt.Errorf("%w: 还有 %d/%d 项验收未完成；如确需归港，用 --force 并写明理由",
					ErrResultRequired, total-doneN, total)
			case total == 0 && result == "":
				return fmt.Errorf("%w: 没有验收项时归港必须填写交付结果；特殊情况用 --force 并写明理由",
					ErrResultRequired)
			}
		}

		now := time.Now()
		if result != "" {
			r.Body = appendLog(r.Body, now, "归港结果（"+by+"）："+result)
		}
		if force {
			r.Body = appendLog(r.Body, now, "⚠️ 强制归港（"+by+"），理由："+reason)
		}
		r.Meta.Status = StatusDone
		r.Meta.CompletedAt = &now
		touch(&r.Meta)
		r.Body = appendLog(r.Body, now, "done by "+by)
		if err := v.rewrite(path, r); err != nil {
			return err
		}
		rec = r
		return nil
	})
	if err != nil {
		return rec, err
	}
	_ = v.ClearActorTask(by)
	return rec, nil
}

// Publish 发布草稿：assignee 为空 → anchored（锚地待派，船看不到）；
// assignee 非空 → ready（直接派船，只有被指派者能接令）。
// berth 非空时同时指定船位（项目目录），只有该目录的会话能接令。
func (v *Vault) Publish(id, by, assignee, berth string) (*Record, error) {
	assignee = strings.TrimSpace(assignee)
	berth = strings.TrimSpace(berth)
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		if r.Meta.Status != StatusDraft {
			return fmt.Errorf("%w: %s 当前状态为 %s，只有草稿可发布", ErrInvalidTransition, id, r.Meta.Status)
		}
		now := time.Now()
		note := ""
		if assignee == "" {
			r.Meta.Status = StatusAnchored
			r.Meta.AssignCwd = ""
			note = "published to anchorage by " + by + "（锚地待派）"
		} else {
			a := assignee
			r.Meta.Assignee = &a
			r.Meta.Status = StatusReady
			r.Meta.AssignCwd = berth
			note = "dispatched by " + by + "，派船 → " + assignee + berthNote(berth)
		}
		touch(&r.Meta)
		r.Body = appendLog(r.Body, now, note)
		if err := v.rewrite(path, r); err != nil {
			return err
		}
		rec = r
		return nil
	})
	return rec, err
}

// Assign 派船/改派：anchored → ready；ready 换船（重新待接令）；
// blocked 可改派给别的船（原船受阻，货回码头待新船接令）。
// in_progress 的货在海上，不能改派（先等它归港）。
// berth 非空表示点名某个项目目录的会话（船位）；为空表示该船任意会话可接。
func (v *Vault) Assign(id, by, assignee, berth string) (*Record, error) {
	assignee = strings.TrimSpace(assignee)
	berth = strings.TrimSpace(berth)
	if assignee == "" {
		return nil, errors.New("派船必须指定执行者（或先发布到锚地）")
	}
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		wasBlocked := false
		switch r.Meta.Status {
		case StatusAnchored:
		case StatusReady:
		case StatusBlocked:
			wasBlocked = true
		default:
			return fmt.Errorf("%w: %s 当前状态为 %s，只有锚地/待接令/受阻的任务可派船改派",
				ErrInvalidTransition, id, r.Meta.Status)
		}
		old := r.Meta.AssigneeValue()
		a := assignee
		r.Meta.Assignee = &a
		r.Meta.AssignCwd = berth
		r.Meta.Status = StatusReady
		r.Meta.BlockedAt = nil
		now := time.Now()
		touch(&r.Meta)
		bn := berthNote(berth)
		switch {
		case old == "":
			r.Body = appendLog(r.Body, now, "dispatched by "+by+"，派船 → "+assignee+bn)
		case wasBlocked:
			r.Body = appendLog(r.Body, now, "reassigned by "+by+"（原船受阻）："+old+" → "+assignee+bn)
		default:
			r.Body = appendLog(r.Body, now, "reassigned by "+by+"："+old+" → "+assignee+bn)
		}
		if err := v.rewrite(path, r); err != nil {
			return err
		}
		rec = r
		return nil
	})
	return rec, err
}

// Block 受阻上浮：in_progress → blocked，必须给原因；只有承担船可操作。
func (v *Vault) Block(id, reason, by string) (*Record, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("阻塞原因不能为空")
	}
	return v.transition(id, "block", strings.TrimSpace(reason), by)
}

// Unblock 解除受阻：blocked → in_progress。
func (v *Vault) Unblock(id, by string) (*Record, error) {
	return v.transition(id, "unblock", "", by)
}

// requireAssignee 校验 by 是任务的承担船。
func requireAssignee(r *Record, by string) error {
	a := r.Meta.AssigneeValue()
	if a == "" {
		return fmt.Errorf("%w: %s 还在锚地未派船", ErrNotAssignee, r.Meta.ID)
	}
	if a != by {
		return fmt.Errorf("%w: %s 派给了 %s，不是 %s", ErrNotAssignee, r.Meta.ID, a, by)
	}
	return nil
}

// requireBerth 校验当前会话目录就是派船时指定的船位。
// 任务没指定船位（AssignCwd 为空）时任意会话可接；远程船 ssh 进对的目录即可。
func requireBerth(r *Record, cwd string) error {
	if r.Meta.AssignCwd == "" || r.Meta.AssignCwd == cwd {
		return nil
	}
	cur := strings.TrimSpace(cwd)
	if cur == "" {
		cur = "未知目录"
	}
	return fmt.Errorf("%w: %s 指定由「%s」项目里的会话接令，当前会话在「%s」",
		ErrWrongBerth, r.Meta.ID, filepath.Base(r.Meta.AssignCwd), filepath.Base(cur))
}

// berthNote 是派船日志里的船位后缀。
func berthNote(berth string) string {
	if berth == "" {
		return ""
	}
	return "（船位：" + filepath.Base(berth) + "）"
}

// acceptanceCount 数「验收标准」区内非空复选框的完成数/总数。
func acceptanceCount(body string) (done, total int) {
	for _, sec := range Sections(body) {
		if sec.Title != "验收标准" {
			continue
		}
		for _, line := range strings.Split(sec.Content, "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "- [x] ") || strings.HasPrefix(t, "- [X] ") {
				if strings.TrimSpace(t[6:]) != "" {
					total++
					done++
				}
			} else if strings.HasPrefix(t, "- [ ] ") {
				if strings.TrimSpace(t[6:]) != "" {
					total++
				}
			}
		}
	}
	return
}

func (v *Vault) transition(id, action, arg, by string) (*Record, error) {
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		now := time.Now()
		touch(&r.Meta)
		switch action {
		case "claim":
			if r.Meta.Status != StatusReady {
				return fmt.Errorf("%w: %s 当前状态为 %s，只有待接令的任务可接令", ErrInvalidTransition, id, r.Meta.Status)
			}
			if err := requireAssignee(r, by); err != nil {
				return err
			}
			r.Meta.Status = StatusInProgress
			r.Meta.ClaimedAt = &now
			r.Body = appendLog(r.Body, now, "claimed by "+by)
		case "block":
			if r.Meta.Status != StatusInProgress {
				return fmt.Errorf("%w: %s 当前状态为 %s，只有进行中的任务可上报受阻", ErrInvalidTransition, id, r.Meta.Status)
			}
			if err := requireAssignee(r, by); err != nil {
				return err
			}
			r.Meta.Status = StatusBlocked
			r.Meta.BlockedAt = &now
			r.Body = appendLog(r.Body, now, "blocked by "+by+"："+arg)
		case "unblock":
			if r.Meta.Status != StatusBlocked {
				return fmt.Errorf("%w: %s 当前状态为 %s，并未受阻", ErrInvalidTransition, id, r.Meta.Status)
			}
			if err := requireAssignee(r, by); err != nil {
				return err
			}
			r.Meta.Status = StatusInProgress
			r.Meta.BlockedAt = nil
			r.Body = appendLog(r.Body, now, "unblocked by "+by)
		default:
			return fmt.Errorf("未知动作 %q", action)
		}
		if err := v.rewrite(path, r); err != nil {
			return err
		}
		rec = r
		return nil
	})
	return rec, err
}

// Log 在任务日志区追加一条过程记录（agent 同步进展的主要方式）。
func (v *Vault) Log(id, line, by string) (*Record, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, errors.New("日志内容不能为空")
	}
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		touch(&r.Meta)
		if by != "" {
			line = by + "：" + line
		}
		r.Body = appendLog(r.Body, time.Now(), line)
		if err := v.rewrite(path, r); err != nil {
			return err
		}
		rec = r
		return nil
	})
	return rec, err
}

// CheckItem 勾选/取消「验收标准」第 n 项（1-based，只数复选框行）。
// 返回勾选后的完成数/总数。
func (v *Vault) CheckItem(id string, n int, checked bool, by string) (done, total int, rec *Record, err error) {
	err = v.withLock(func() error {
		path, e := v.findTaskPath(id)
		if e != nil {
			return e
		}
		r, e := parseFile(path)
		if e != nil {
			return e
		}
		body, d, t, e := setCheckbox(r.Body, n, checked)
		if e != nil {
			return e
		}
		r.Body = body
		touch(&r.Meta)
		if checked {
			r.Body = appendLog(r.Body, time.Now(),
				fmt.Sprintf("验收项 %d/%d 完成%s", n, t, actorSuffix(by)))
		}
		if e = v.rewrite(path, r); e != nil {
			return e
		}
		done, total, rec = d, t, r
		return nil
	})
	return
}

// UpdateTask 编辑标题/正文/重要程度/目标；标题变化时文件同步改名，T 编号不变。
func (v *Vault) UpdateTask(id, title, body, importance, goal string) (*Record, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("任务标题不能为空")
	}
	if !ValidImportance(importance) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidImportance, importance)
	}
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		// 派船即封舱：草稿/锚地阶段可随意打磨，一旦派船，验收标准以开船时为准。
		if r.Meta.Status != StatusDraft && r.Meta.Status != StatusAnchored {
			return fmt.Errorf("%w: %s 已派船（%s），封舱后不能改单；要调整请批注或改派",
				ErrInvalidTransition, id, r.Meta.Status)
		}
		r.Meta.Title = title
		r.Meta.Importance = importance
		r.Meta.Goal = strings.TrimSpace(goal)
		r.Body = strings.TrimLeft(body, "\n")
		touch(&r.Meta)
		newPath := filepath.Join(v.Root, "tasks", id+"-"+slug(title)+".md")
		if err := v.write(newPath, r.Meta, r.Body); err != nil {
			return err
		}
		if newPath != path {
			_ = os.Remove(path)
		}
		r.Path = newPath
		rec = r
		return nil
	})
	return rec, err
}

func actorSuffix(by string) string {
	if by == "" {
		return ""
	}
	return "（" + by + "）"
}

// Next 返回派给某艘船、待它接令的最重要一个任务（重要程度升序，同级按编号）。
// 锚地里未派船的货对船不可见；指定了船位（assign_cwd）的货只在对应项目目录出现。
func (v *Vault) Next(actor, cwd string) (*Record, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, errors.New("需要 --by 指定船名（谁在接令？）")
	}
	tasks, err := v.ListTasks(StatusReady)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		m := tasks[i].Meta
		if m.AssigneeValue() == actor && (m.AssignCwd == "" || m.AssignCwd == cwd) {
			return &tasks[i], nil
		}
	}
	return nil, fmt.Errorf("%w: 没有派给 %s 的待接令任务", ErrNotFound, actor)
}

// GetTask 按编号取单个任务。
func (v *Vault) GetTask(id string) (*Record, error) {
	var r *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		rec, err := parseFile(path)
		if err != nil {
			return err
		}
		r = rec
		return nil
	})
	return r, err
}

// DeleteTask 删除还没出港的任务文件（draft 备货中 / anchored 锚地待派可删；
// ready 及以后有派船与流转历史，不能直接抹掉，要撤请批注/改派/归港）。
func (v *Vault) DeleteTask(id string) error {
	return v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		rec, err := parseFile(path)
		if err != nil {
			return err
		}
		if rec.Meta.Status != StatusDraft && rec.Meta.Status != StatusAnchored {
			return fmt.Errorf("%w: %s 当前为 %s，已派船出港的货不能直接删", ErrNotDraft, id, rec.Meta.Status)
		}
		return os.Remove(path)
	})
}

// ListTasks 列出任务，可按状态过滤（空串=全部），按重要程度再按编号升序。
func (v *Vault) ListTasks(status string) ([]Record, error) {
	all, err := listDir(filepath.Join(v.Root, "tasks"))
	if err != nil {
		return nil, err
	}
	if status != "" {
		out := all[:0]
		for _, r := range all {
			if r.Meta.Status == status {
				out = append(out, r)
			}
		}
		all = out
	}
	sort.Slice(all, func(i, j int) bool {
		ri, rj := importanceRank[all[i].Meta.Importance], importanceRank[all[j].Meta.Importance]
		if ri != rj {
			return ri < rj
		}
		return all[i].Meta.ID < all[j].Meta.ID
	})
	return all, nil
}

func (v *Vault) findTaskPath(id string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(v.Root, "tasks"))
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), id+"-") && strings.HasSuffix(e.Name(), ".md") {
			return filepath.Join(v.Root, "tasks", e.Name()), nil
		}
	}
	return "", fmt.Errorf("%w: 没有任务 %s", ErrNotFound, id)
}

func (v *Vault) nextID() (string, error) {
	p := filepath.Join(v.Root, ".port", "counter")
	n := 0
	if b, err := os.ReadFile(p); err == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	n++
	if err := os.WriteFile(p, []byte(strconv.Itoa(n)), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("T%04d", n), nil
}

// rewrite 在元数据可能变化后重写任务文件。
func (v *Vault) rewrite(path string, r *Record) error {
	return v.write(path, r.Meta, r.Body)
}

// ---------- 正文工具 ----------

// Sections 把正文按 "## 标题" 切成有序小节（序言的 key 为空串）。
func Sections(body string) []struct{ Title, Content string } {
	var out []struct{ Title, Content string }
	curTitle := ""
	var cur strings.Builder
	flush := func() {
		out = append(out, struct{ Title, Content string }{curTitle, strings.Trim(cur.String(), "\n")})
		cur.Reset()
	}
	for _, line := range strings.Split(body, "\n") {
		if t := strings.TrimPrefix(line, "## "); t != line && !strings.HasPrefix(line, "###") {
			flush()
			curTitle = strings.TrimSpace(t)
			continue
		}
		cur.WriteString(line + "\n")
	}
	flush()
	return out
}

// setCheckbox 操作「验收标准」小节内第 n 个复选框（1-based）。
func setCheckbox(body string, n int, checked bool) (string, int, int, error) {
	lines := strings.Split(body, "\n")
	inAcceptance := false
	idx := 0
	found := false
	done, total := 0, 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inAcceptance = trimmed == "## 验收标准"
			continue
		}
		if !inAcceptance {
			continue
		}
		var rest string
		var isBox bool
		if strings.HasPrefix(trimmed, "- [ ] ") {
			rest = strings.TrimPrefix(trimmed, "- [ ] ")
			isBox = true
		} else if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			rest = trimmed[6:]
			isBox = true
		}
		if !isBox || strings.TrimSpace(rest) == "" {
			continue
		}
		total++
		{
			idx++
			if strings.HasPrefix(strings.ToLower(trimmed), "- [x]") {
				done++
			}
			if idx == n {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				mark := " "
				if checked {
					mark = "x"
					done++
				}
				lines[i] = indent + "- [" + mark + "] " + rest
				found = true
			}
		}
	}
	if !found {
		return "", 0, 0, fmt.Errorf("验收项 #%d 不存在（空复选框不计）", n)
	}
	return strings.Join(lines, "\n"), done, total, nil
}

// ---------- 文件编解码 ----------

func parseFile(path string) (*Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)
	r := &Record{Path: path}
	if !strings.HasPrefix(text, "---\n") {
		return nil, fmt.Errorf("%s: 缺少 frontmatter", path)
	}
	rest0 := text[4:]
	end := strings.Index(rest0, "\n---")
	if end < 0 {
		return nil, fmt.Errorf("%s: frontmatter 未闭合", path)
	}
	if err := yaml.Unmarshal([]byte(rest0[:end]), &r.Meta); err != nil {
		return nil, fmt.Errorf("%s: frontmatter 解析失败: %w", path, err)
	}
	canonicalize(&r.Meta)
	// end 指向闭合标记前的换行；end+1 之后以 "---" 开头。
	body := strings.TrimPrefix(rest0[end+1:], "---")
	body = strings.TrimLeft(body, "\r\n")
	r.Body = body
	return r, nil
}

// canonicalize 做旧字段迁移与缺省值补齐。
func canonicalize(m *Meta) {
	if m.Importance == "" {
		m.Importance = m.Priority
		m.Priority = ""
	}
	if m.Importance == "" {
		if m.Type == "task" {
			m.Importance = ImpNormal
		}
	}
	// v0.4 迁移：ready 必须有承担船；旧版「已发布、任领取」的货退回锚地待派。
	if m.Type == "task" && m.Status == StatusReady && m.AssigneeValue() == "" {
		m.Status = StatusAnchored
	}
}

func (v *Vault) write(path string, m Meta, body string) error {
	canonicalize(&m)
	yb, err := yaml.Marshal(&m)
	if err != nil {
		return err
	}
	// yaml 库在某些字段组合下会附加文档标记行（--- / ...），统一剥掉，
	// 由我们自己写唯一一对 frontmatter 分隔线。
	for {
		yb = bytes.TrimRight(yb, "\n")
		switch {
		case bytes.HasSuffix(yb, []byte("\n---")), bytes.HasSuffix(yb, []byte("\n...")):
			yb = yb[:len(yb)-4]
		case bytes.Equal(yb, []byte("---")), bytes.Equal(yb, []byte("...")):
			yb = nil
		default:
			goto markersDone
		}
	}
markersDone:
	if bytes.HasPrefix(yb, []byte("---\n")) {
		yb = bytes.TrimSpace(yb[4:])
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(yb)
	sb.WriteString("\n---\n\n")
	sb.WriteString(strings.TrimLeft(body, "\n"))
	if !strings.HasSuffix(body, "\n") {
		sb.WriteString("\n")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path) // 同目录原子替换
}

// appendLog 在「## 日志」小节末尾（下一个 ## 标题之前，否则正文末尾）
// 按时间顺序追加一行。
func appendLog(body string, at time.Time, line string) string {
	entry := "- " + at.Format("2006-01-02 15:04 -0700") + " — " + line + "\n"
	idx := strings.Index(body, "## 日志")
	if idx < 0 {
		return strings.TrimRight(body, "\n") + "\n\n## 日志\n" + entry
	}
	tail := body[idx+len("## 日志"):]
	if rel := strings.Index(tail, "\n## "); rel >= 0 {
		cut := idx + len("## 日志") + rel
		return body[:cut] + "\n" + entry + body[cut:]
	}
	return strings.TrimRight(body, "\n") + "\n" + entry
}

func (v *Vault) uniquePath(path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path, nil
	} else if err != nil {
		return "", err
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		p := fmt.Sprintf("%s-%d%s", stem, i, ext)
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return p, nil
		} else if err != nil {
			return "", err
		}
	}
}

func listDir(dir string) ([]Record, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		r, err := parseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // 损坏文件跳过，doctor 将来负责报告
		}
		out = append(out, *r)
	}
	return out, nil
}

// slug 保留中英文与数字（中文标题不能被清空），其余折叠为 -。
func slug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r >= 0x4E00 && r <= 0x9FFF:
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if utf8.RuneCountInString(out) > 40 {
		out = string([]rune(out)[:40])
	}
	return out
}
