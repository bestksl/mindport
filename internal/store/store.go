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

// 任务状态（ADR-0003 状态机）。
const (
	StatusReady      = "ready"
	StatusInProgress = "in_progress"
	StatusBlocked    = "blocked"
	StatusDone       = "done"
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

const fileTimeLayout = "20060102-1504"

// Open 打开（必要时初始化）vault 的标准目录结构。
func Open(root string) (*Vault, error) {
	for _, d := range []string{
		"inbox", "tasks", "notes", "archive", "attachments",
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

// ---------- 闪念 ----------

// AddThought 往 inbox/ 丢一条闪念，三秒捕获，不分配编号。
func (v *Vault) AddThought(title, body, source string) (*Record, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("闪念内容不能为空")
	}
	var rec *Record
	err := v.withLock(func() error {
		now := time.Now()
		m := Meta{Type: "thought", Title: title, Created: now, Source: source}
		base := now.Format(fileTimeLayout) + "-" + slug(title)
		path, err := v.uniquePath(filepath.Join(v.Root, "inbox", base+".md"))
		if err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			body = title
		}
		if err := v.write(path, m, body); err != nil {
			return err
		}
		rec = &Record{Meta: m, Body: body, Path: path}
		return nil
	})
	return rec, err
}

// ListThoughts 列出 inbox，新的在前。
func (v *Vault) ListThoughts() ([]Record, error) {
	out, err := listDir(filepath.Join(v.Root, "inbox"))
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[j].Meta.Created.Before(out[i].Meta.Created)
	})
	return out, nil
}

// PromoteThought 把 inbox 里的闪念转成正式任务：分配 T 编号、写入模板、
// 原正文移入「描述」，闪念文件删除。
func (v *Vault) PromoteThought(filename, importance string) (*Record, error) {
	if !ValidImportance(importance) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidImportance, importance)
	}
	var rec *Record
	err := v.withLock(func() error {
		src := filepath.Join(v.Root, "inbox", filepath.Base(filename))
		thought, err := parseFile(src)
		if err != nil {
			return fmt.Errorf("闪念不存在或无法解析: %w", err)
		}
		now := time.Now()
		id, err := v.nextID()
		if err != nil {
			return err
		}
		imp := importance
		if imp == "" {
			imp = ImpNormal
		}
		m := Meta{
			Type: "task", ID: id, Title: thought.Meta.Title,
			Status: StatusReady, Importance: imp,
			Created: now,
		}
		desc := strings.TrimSpace(thought.Body)
		body := "## 描述\n\n" + desc + "\n\n## 验收标准\n\n## 日志\n" +
			"- " + now.Format("2006-01-02 15:04 -0700") +
			" — promoted from inbox/" + filepath.Base(src) + "\n"
		path, err := v.uniquePath(filepath.Join(v.Root, "tasks", id+"-"+slug(thought.Meta.Title)+".md"))
		if err != nil {
			return err
		}
		if err := v.write(path, m, body); err != nil {
			return err
		}
		if err := os.Remove(src); err != nil {
			return err
		}
		rec = &Record{Meta: m, Body: body, Path: path}
		return nil
	})
	return rec, err
}

// ---------- 任务 ----------

// NewTask 创建一个 ready 任务并分配 T 编号。
func (v *Vault) NewTask(title, importance, goal string) (*Record, error) {
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
		m := Meta{
			Type: "task", ID: id, Title: title,
			Status: StatusReady, Importance: imp, Goal: strings.TrimSpace(goal),
			Created: now,
		}
		body := "## 描述\n\n" + title + "\n\n## 验收标准\n\n## 日志\n" +
			"- " + now.Format("2006-01-02 15:04 -0700") + " — created by port\n"
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

// Claim 领取任务：ready → in_progress。
func (v *Vault) Claim(id, by string) (*Record, error) {
	return v.transition(id, "claim", "", by)
}

// Done 完成任务：in_progress（或 blocked 显式归港）→ done。
func (v *Vault) Done(id, by string) (*Record, error) {
	return v.transition(id, "done", "", by)
}

// Block 阻塞任务：in_progress → blocked，必须给原因。
func (v *Vault) Block(id, reason, by string) (*Record, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("阻塞原因不能为空")
	}
	return v.transition(id, "block", strings.TrimSpace(reason), by)
}

// Unblock 解除阻塞：blocked → in_progress。
func (v *Vault) Unblock(id, by string) (*Record, error) {
	return v.transition(id, "unblock", "", by)
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
				return fmt.Errorf("%w: %s 当前状态为 %s，只有 ready 可领取", ErrInvalidTransition, id, r.Meta.Status)
			}
			r.Meta.Status = StatusInProgress
			r.Meta.Assignee = &by
			r.Meta.ClaimedAt = &now
			r.Body = appendLog(r.Body, now, "claimed by "+by)
		case "done":
			if r.Meta.Status != StatusInProgress && r.Meta.Status != StatusBlocked {
				return fmt.Errorf("%w: %s 当前状态为 %s，只有进行中/受阻的任务可归港", ErrInvalidTransition, id, r.Meta.Status)
			}
			r.Meta.Status = StatusDone
			r.Meta.CompletedAt = &now
			if r.Meta.Assignee == nil {
				r.Meta.Assignee = &by
			}
			r.Body = appendLog(r.Body, now, "done by "+by)
		case "block":
			if r.Meta.Status != StatusInProgress {
				return fmt.Errorf("%w: %s 当前状态为 %s，只有进行中的任务可阻塞", ErrInvalidTransition, id, r.Meta.Status)
			}
			r.Meta.Status = StatusBlocked
			r.Meta.BlockedAt = &now
			r.Body = appendLog(r.Body, now, "blocked by "+by+"："+arg)
		case "unblock":
			if r.Meta.Status != StatusBlocked {
				return fmt.Errorf("%w: %s 当前状态为 %s，并未受阻", ErrInvalidTransition, id, r.Meta.Status)
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

// Next 返回当前最重要的一个 ready 任务（重要程度升序，同级按编号）。
func (v *Vault) Next() (*Record, error) {
	tasks, err := v.ListTasks(StatusReady)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("%w: 港里没有待领取的任务", ErrNotFound)
	}
	return &tasks[0], nil
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
