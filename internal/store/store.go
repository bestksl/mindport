// Package store 是思港的文件存储层：vault 中的每个对象都是一个
// 「YAML frontmatter + Markdown 正文」文件。所有写操作经同一把
// flock 串行化（ADR-0003）。
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
	Priority    string     `yaml:"priority,omitempty" json:"priority,omitempty"`
	Assignee    *string    `yaml:"assignee,omitempty" json:"assignee,omitempty"`
	Tags        []string   `yaml:"tags,omitempty" json:"tags,omitempty"`
	Created     time.Time  `yaml:"created" json:"created"`
	Updated     *time.Time `yaml:"updated,omitempty" json:"updated,omitempty"`
	ClaimedAt   *time.Time `yaml:"claimed_at,omitempty" json:"claimed_at,omitempty"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty" json:"completed_at,omitempty"`
	Source      string     `yaml:"source,omitempty" json:"source,omitempty"`
}

// Record 是一个从磁盘读出（或待写入）的对象。
type Record struct {
	Meta Meta
	Body string
	Path string
}

// 任务状态（ADR-0003 状态机的 v1 子集）。
const (
	StatusReady      = "ready"
	StatusInProgress = "in_progress"
	StatusBlocked    = "blocked"
	StatusDone       = "done"
	StatusCancelled  = "cancelled"
)

var (
	// ErrNotFound 表示按编号找不到任务。
	ErrNotFound = errors.New("not found")
	// ErrInvalidTransition 表示状态跳转不合法。
	ErrInvalidTransition = errors.New("invalid status transition")
)

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

// ---------- 任务 ----------

// NewTask 创建一个 ready 任务并分配 T 编号（add + promote 一步到位）。
func (v *Vault) NewTask(title string) (*Record, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("任务标题不能为空")
	}
	var rec *Record
	err := v.withLock(func() error {
		now := time.Now()
		id, err := v.nextID()
		if err != nil {
			return err
		}
		m := Meta{
			Type: "task", ID: id, Title: title,
			Status: StatusReady, Priority: "normal",
			Created: now,
		}
		body := "## 描述\n\n" + title + "\n\n## 验收标准\n\n- [ ] \n\n## 日志\n" +
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
	return v.transition(id, "claim", by)
}

// Done 完成任务：in_progress → done。
func (v *Vault) Done(id, by string) (*Record, error) {
	return v.transition(id, "done", by)
}

func (v *Vault) transition(id, action, by string) (*Record, error) {
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
		r.Meta.Updated = &now
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
			if r.Meta.Status != StatusInProgress {
				return fmt.Errorf("%w: %s 当前状态为 %s，只有 in_progress 可完成", ErrInvalidTransition, id, r.Meta.Status)
			}
			r.Meta.Status = StatusDone
			r.Meta.CompletedAt = &now
			if r.Meta.Assignee == nil {
				r.Meta.Assignee = &by
			}
			r.Body = appendLog(r.Body, now, "done by "+by)
		default:
			return fmt.Errorf("未知动作 %q", action)
		}
		if err := v.write(path, r.Meta, r.Body); err != nil {
			return err
		}
		rec = r
		return nil
	})
	return rec, err
}

// ListTasks 列出任务，可按状态过滤（空串=全部），按 T 编号升序。
func (v *Vault) ListTasks(status string) ([]Record, error) {
	all, err := listDir(filepath.Join(v.Root, "tasks"))
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Meta.ID < all[j].Meta.ID })
	if status == "" {
		return all, nil
	}
	out := all[:0]
	for _, r := range all {
		if r.Meta.Status == status {
			out = append(out, r)
		}
	}
	return out, nil
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
	// end 指向闭合标记前的换行；end+1 之后以 "---" 开头。
	body := strings.TrimPrefix(rest0[end+1:], "---")
	body = strings.TrimLeft(body, "\r\n")
	r.Body = body
	return r, nil
}

func (v *Vault) write(path string, m Meta, body string) error {
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
