// 目录扫盘：不依赖心跳/官方发现口，直接列出某个项目目录下的全部 Claude 会话
// transcript（含早已退出的历史会话）。简单粗暴——glob + 逐文件轻量扫描，
// 只取标题/时间/计数，不跑块级解析。
package transcript

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// listMaxFiles 防止异常目录（全磁盘编码碰撞等极端情况）扫描失控。
const listMaxFiles = 300

// recentHeadBytes / recentTailBytes：聚合时每个文件只读头、尾各这么多字节。
// 真人首句在开头；当前目录在最后一条记录的 cwd（会话可能跨目录 --resume，
// 头部 cwd 已过期，projects 编码目录对应的是最后/当前 cwd）。
const recentHeadBytes = 64 * 1024
const recentTailBytes = 64 * 1024

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

// RecentSessions 扫 ~/.claude/projects 下全部编码目录，聚合最近活跃的会话。
// 不依赖心跳：编码目录名不可逆，但 transcript 每条消息带顶层 cwd，
// 真实项目路径由文件内容自证恢复。scanFiles 限制扫描文件数（按 mtime 取最新），
// total 限制返回条数，避免历史目录巨大时读爆。
func RecentSessions(home string, scanFiles, total int) ([]SessionMeta, error) {
	root := filepath.Join(home, ".claude", "projects")
	projDirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	type cand struct {
		path string
		mod  time.Time
	}
	var cands []cand
	for _, d := range projDirs {
		if !d.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			cands = append(cands, cand{filepath.Join(root, d.Name(), f.Name()), info.ModTime()})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.After(cands[j].mod) })
	if len(cands) > scanFiles {
		cands = cands[:scanFiles]
	}
	out := make([]SessionMeta, 0, len(cands))
	for _, c := range cands {
		// 聚合只需标题（文件头）+ 当前目录与末事件时间（文件尾）：
		// 各读 64KB，不逐行扫几十上百 MB 的全文，也不做消息计数。
		m := scanMetaCheap(c.path)
		if m.Dir == "" {
			continue // 老格式无 cwd，无法自证路径，不展示
		}
		m.SID = strings.TrimSuffix(filepath.Base(c.path), ".jsonl")
		m.ModTime = c.mod
		if m.LastTS.IsZero() {
			m.LastTS = c.mod
		}
		out = append(out, m)
		if len(out) >= total {
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

// MetaPath 按 transcript 文件绝对路径取摘要（目录编码不可逆、只拿得到
// 编码目录时用这个）；Dir 以记录内 cwd 自证，SID 取自文件名。
func MetaPath(path string) SessionMeta {
	m := scanMeta(path)
	m.SID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return m
}

// metaLine 只解目录摘要需要的字段，避开块级解析开销。
type metaLine struct {
	Type                 string          `json:"type"`
	Timestamp            string          `json:"timestamp"`
	CWD                  string          `json:"cwd"`
	IsSidechain          bool            `json:"isSidechain"`
	InterruptedMessageID string          `json:"interruptedMessageId"`
	IsCompactSummary     bool            `json:"isCompactSummary"`
	Message              json.RawMessage `json:"message"`
}

// isSystemMarker：中断/压缩续接是状态记录，不算一条消息，也不能当标题。
func (l metaLine) isSystemMarker() bool {
	return l.InterruptedMessageID != "" || l.IsCompactSummary
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
		if !rec.isSystemMarker() && !(rec.Type == "user" && injectionOnly(rec.Message)) {
			m.Events++ // 中断/压缩与系统注入是状态/噪声，不计入消息条数
		}
		ts := ParseTS(rec.Timestamp)
		if m.FirstTS.IsZero() {
			m.FirstTS = ts
		}
		m.LastTS = ts
		// 最后一条消息的 cwd 即会话当前目录（worktree 切换等历史路径被覆盖）
		if rec.CWD != "" {
			m.Dir = rec.CWD
		}
		if m.Title == "" && rec.Type == "user" && !rec.IsSidechain && !rec.isSystemMarker() {
			if t := firstHumanText(rec.Message); titleWorthy(t) {
				m.Title = clip(t, 60)
			}
		}
	}
	return m
}

// scanMetaCheap 只读文件头、尾各一小段：头部找第一条真人发言当标题，
// 尾部找最后一条带 cwd 的记录（当前项目路径，跨目录 resume 的会话头部
// cwd 已过期）与最后时间。用于聚合最近会话，避免全文扫描大文件。
func scanMetaCheap(path string) SessionMeta {
	var m SessionMeta
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()
	info, _ := f.Stat()

	scan := func(r io.Reader, head bool) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}
			var rec metaLine
			if json.Unmarshal(line, &rec) != nil {
				continue // 尾部从行中间开始，第一行大概率解析失败
			}
			// 任何带 cwd 的记录都算数，尾部后出现的覆盖成当前目录
			if rec.CWD != "" {
				m.Dir = rec.CWD
			}
			if ts := ParseTS(rec.Timestamp); !ts.IsZero() {
				m.LastTS = ts
				if m.FirstTS.IsZero() {
					m.FirstTS = ts
				}
			}
			if head && m.Title == "" && rec.Type == "user" && !rec.IsSidechain && !rec.isSystemMarker() {
				if t := firstHumanText(rec.Message); titleWorthy(t) {
					m.Title = clip(t, 60)
				}
			}
		}
	}

	// 头部
	scan(io.LimitReader(f, recentHeadBytes), true)
	// 尾部（小文件已全在头部覆盖；大文件 seek 到尾部，首行截断会被容错跳过）
	if info != nil && info.Size() > recentHeadBytes {
		if _, err := f.Seek(info.Size()-recentTailBytes, io.SeekStart); err == nil {
			scan(f, false)
		}
	}
	return m
}

// titleWorthy 排除系统注入的「伪首句」：compaction 续接提示、
// 图片占位说明、任务通知 XML 等都不是真人起的话，不该当会话标题。
func titleWorthy(t string) bool {
	t = strings.TrimSpace(t)
	switch {
	case t == "":
		return false
	case strings.HasPrefix(t, "This session is being continued"):
		return false
	case strings.HasPrefix(t, "[Request interrupted"):
		return false
	case strings.HasPrefix(t, "[Image:"):
		return false
	case IsSystemInjection(t):
		return false
	}
	return true
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
		// 跳过任务通知等系统注入块，标题必须取自真人写的那句。
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" && !IsSystemInjection(b.Text) {
			return b.Text
		}
	}
	return ""
}
