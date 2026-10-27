// 目录扫盘：不依赖心跳/官方发现口，直接列出某个项目目录下的全部 Claude 会话
// transcript（含早已退出的历史会话）。简单粗暴——glob + 逐文件轻量扫描，
// 只取标题/时间/计数，不跑块级解析。
package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// listMaxFiles 防止异常目录（全磁盘编码碰撞等极端情况）扫描失控。
const listMaxFiles = 300

// SessionMeta 是一个会话文件的目录级摘要。
type SessionMeta struct {
	SID     string    `json:"sid"`
	Dir     string    `json:"dir"`
	Title   string    `json:"title"` // 第一条真人发言（截断）
	FirstTS time.Time `json:"-"`
	LastTS  time.Time `json:"-"`
	Events  int       `json:"events"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"-"`
}

// ProjectDir 返回某项目 cwd 对应的 transcript 目录（~/.claude/projects/<编码>）。
func ProjectDir(home, cwd string) string {
	var b strings.Builder
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return filepath.Join(home, ".claude", "projects", b.String())
}

// ListSessions 扫描 dir 目录下全部 *.jsonl 会话，按最后更新倒序返回。
// 目录不存在返回 ErrNotFound。
func ListSessions(home, dir string) ([]SessionMeta, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, ErrNotFound
	}
	pdir := ProjectDir(home, dir)
	entries, err := os.ReadDir(pdir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	out := make([]SessionMeta, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sid := strings.TrimSuffix(e.Name(), ".jsonl")
		full := filepath.Join(pdir, e.Name())
		meta := scanMeta(full)
		meta.SID = sid
		meta.Dir = dir
		if info, err := e.Info(); err == nil {
			meta.Size = info.Size()
			meta.ModTime = info.ModTime()
			if meta.LastTS.IsZero() {
				meta.LastTS = info.ModTime()
			}
		}
		out = append(out, meta)
		if len(out) >= listMaxFiles {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastTS.After(out[j].LastTS) })
	return out, nil
}

// Meta 取单个会话的目录级摘要（标题/时间/计数）；文件不存在时零值返回。
func Meta(home, dir, sid string) SessionMeta {
	m := scanMeta(Path(home, dir, sid))
	m.SID = sid
	m.Dir = dir
	return m
}

// metaLine 只解目录摘要需要的字段，避开块级解析开销。
type metaLine struct {
	Type        string          `json:"type"`
	Timestamp   string          `json:"timestamp"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

// scanMeta 顺序扫一个文件：事件计数、首末时间、第一条真人发言当标题。
func scanMeta(path string) SessionMeta {
	var m SessionMeta
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024) // 单行最大 2MB
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec metaLine
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if rec.Type != "user" && rec.Type != "assistant" || len(rec.Message) == 0 {
			continue
		}
		m.Events++
		ts := ParseTS(rec.Timestamp)
		if m.FirstTS.IsZero() {
			m.FirstTS = ts
		}
		m.LastTS = ts
		if m.Title == "" && rec.Type == "user" && !rec.IsSidechain {
			if t := firstHumanText(rec.Message); t != "" {
				m.Title = clip(t, 60)
			}
		}
	}
	return m
}

// firstHumanText 从 user 消息里取真人文本：先剥 message 外壳拿 content，
// content 直接是字符串就用，否则取内容数组里第一个 type=text 的块
// （纯工具回报的 user 消息没有 text 块）。
func firstHumanText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil || len(msg.Content) == 0 {
		return ""
	}
	c := msg.Content
	if c[0] == '"' {
		var s string
		if json.Unmarshal(c, &s) == nil {
			return s
		}
		return ""
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(c, &blocks) != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			return b.Text
		}
	}
	return ""
}
