// Package transcript 只读桥接 Claude Code 的会话 transcript（~/.claude/projects/.../*.jsonl）：
// 定位文件、按字节范围增量解析、输出调度台可渲染的事件流。详见 ADR-0008。
//
// 这是未成文但事实上稳定的本地接口：解析严格容错——坏行跳过、字段缺席降级，
// 格式漂移最坏结果是某些块不显示，绝不影响思港主链路。本包不写、不改写 transcript。
package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 块大小与截断阈值。
const (
	tailBytes  = 128 << 10 // 首屏读最后 128KB
	chunkBytes = 128 << 10 // 每次上翻 128KB
	maxIncRead = 2 << 20   // 单次增量最多读 2MB（防异常暴涨）
	maxText    = 8 << 10   // thinking 原文保留上限
	maxDetail  = 4 << 10   // 工具输入/结果原文保留上限
	maxSummary = 200
)

// ErrNotFound 表示按会话定位不到 transcript 文件（会话太旧或已被清理）。
var ErrNotFound = errors.New("transcript not found")

// injectionTags 是 Claude Code 以 role=user 身份落盘、但并非任何人发言的
// 系统注入文本：后台任务完成/中止通知、系统提醒、斜杠命令包装、本地命令
// 回报、超长输出指引等。它们一旦按用户气泡渲染，就会在真人那句话上方
// 冒出一段结构化 XML——豆包式视图必须整条滤掉。
var injectionTags = []string{
	"<task-notification>",
	"<system-reminder>",
	"<command-name>",
	"<command-message>",
	"<command-args>",
	"<local-command-request>",
	"<local-command-stdout>",
	"<local-command-stderr>",
	"<persisted-output>",
	"<bash-input>",
	"<bash-stdout>",
	"<bash-stderr>",
	"<bash-metadata>",
}

// IsSystemInjection 报告一段文本是不是系统注入（而非真人发言）。
// 只认行首标签，避免误伤正文里引用了 XML 的真人消息。
func IsSystemInjection(s string) bool {
	t := strings.TrimLeft(s, " \t\r\n")
	for _, tag := range injectionTags {
		if strings.HasPrefix(t, tag) {
			return true
		}
	}
	return false
}

// injectionOnly 报告一条 user 记录是否除系统注入文本外没有任何真人内容
// （字符串 content 本身是注入，或块数组里只剩注入 text 块）。
// 供目录计数把这类记录与中断/压缩状态记录同等排除。
func injectionOnly(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var msg wireMessage
	if json.Unmarshal(raw, &msg) != nil || len(msg.Content) == 0 {
		return false
	}
	if msg.Content[0] == '"' {
		var s string
		return json.Unmarshal(msg.Content, &s) == nil && strings.TrimSpace(s) != "" && IsSystemInjection(s)
	}
	var blocks []wireBlock
	if json.Unmarshal(msg.Content, &blocks) != nil || len(blocks) == 0 {
		return false
	}
	sawText := false
	for _, b := range blocks {
		if b.Type != "text" {
			return false // 含工具回报等其它块，交给既有折叠逻辑
		}
		if strings.TrimSpace(b.Text) == "" {
			continue
		}
		sawText = true
		if !IsSystemInjection(b.Text) {
			return false
		}
	}
	return sawText
}

// Block 是时间线上的一个内容块。
type Block struct {
	Kind      string `json:"kind"` // text | thinking | tool | result | interrupt | continue
	Text      string `json:"text,omitempty"`
	ToolID    string `json:"tid,omitempty"`     // tool/result：关联 id
	Name      string `json:"name,omitempty"`    // tool：工具名
	Summary   string `json:"summary,omitempty"` // tool：白名单动作摘要
	IsError   bool   `json:"err,omitempty"`     // result（独立兜底块）：失败
	Result    string `json:"result,omitempty"`  // tool：工具回报原文（已按 tid 并入同一张工具卡）
	ResultErr bool   `json:"rerr,omitempty"`    // tool：回报是失败
	HTML      string `json:"html,omitempty"`    // text：server 层填充的安全 Markdown HTML（前端富文本）
	Collapsed bool   `json:"collapsed,omitempty"`
}

// Event 是一条消息（幂等键为 ID=transcript uuid）。
type Event struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"` // user | assistant | system
	TS        string  `json:"ts"`   // RFC3339（UTC 原样，前端本地化）
	Sidechain bool    `json:"side,omitempty"`
	Blocks    []Block `json:"blocks"`
}

// Page 是一次范围读取的结果。
type Page struct {
	Events []Event `json:"events"`
	From   int64   `json:"from"`   // 本批最早完整行的偏移（继续上翻用 before=From）
	Cursor int64   `json:"cursor"` // 下一轮增量起点（ReadSince）
	Size   int64   `json:"size"`   // 文件当前总字节
}

// Path 由 home 目录、会话 cwd、sessionId 定位 transcript 文件。
// 目录编码复刻 Claude Code：非字母数字字符一律替换为 '-'。
func Path(home, cwd, sid string) string {
	var b strings.Builder
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return filepath.Join(home, ".claude", "projects", b.String(), sid+".jsonl")
}

// Exists 报告 transcript 文件是否存在。
func Exists(home, cwd, sid string) bool {
	if cwd == "" || sid == "" {
		return false
	}
	st, err := os.Stat(Path(home, cwd, sid))
	return err == nil && !st.IsDir()
}

// ReadTail 读文件最后至多 tailBytes（行边界对齐），用于首屏。
func ReadTail(path string) (*Page, error) {
	f, size, err := openSize(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	start := size - tailBytes
	if start < 0 {
		start = 0
	}
	events, aligned, next, err := readRange(f, start, size, start > 0)
	if err != nil {
		return nil, err
	}
	return &Page{Events: events, From: aligned, Cursor: next, Size: size}, nil
}

// ReadBefore 读 end 之前至多 chunkBytes 的更早内容（向上翻页）。
func ReadBefore(path string, end int64) (*Page, error) {
	f, size, err := openSize(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if end > size || end <= 0 {
		end = size
	}
	start := end - chunkBytes
	if start < 0 {
		start = 0
	}
	events, aligned, _, err := readRange(f, start, end, start > 0)
	if err != nil {
		return nil, err
	}
	return &Page{Events: events, From: aligned, Size: size}, nil
}

// ReadSince 读 [from, EOF) 的增量（幂等：同一 from 重复调用结果一致）。
// 文件若被截断/换新（size<from）则从头读起。
func ReadSince(path string, from int64) (*Page, error) {
	f, size, err := openSize(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if from > size || from < 0 {
		from = 0
	}
	if size-from > maxIncRead { // 暴涨时只取窗口尾部，其余靠下一轮
		from = size - maxIncRead
	}
	events, _, next, err := readRange(f, from, size, false)
	if err != nil {
		return nil, err
	}
	return &Page{Events: events, From: next, Cursor: next, Size: size}, nil
}

func openSize(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

// readRange 读字节区间 [start,end) 并逐行解析。
//   - 区间起点不在文件头时，第一个（可能残缺的）半行丢弃并把 From 对齐到下一行；
//   - 区间结尾若停在一条还没写完的行上（增量轮询正撞上写盘），该行不解析，
//     返回的游标停在它的行首——下一轮从同一字节重读，事件不会丢、不会重复。
//
// aligned 是本批第一条事件的行首偏移（上翻锚点），next 是增量游标
// （尾部撞上未写完的行时回退到该行行首）。
func readRange(f *os.File, start, end int64, dropHead bool) (events []Event, aligned, next int64, err error) {
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return nil, start, start, err
	}
	br := bufio.NewReader(io.LimitReader(f, end-start))
	pos := start
	next = start
	aligned = start
	first := true
	for {
		seg, rerr := br.ReadBytes('\n')
		lineStart := pos
		pos += int64(len(seg))
		complete := len(seg) > 0 && seg[len(seg)-1] == '\n'
		droppedHead := first && dropHead
		if complete {
			if droppedHead {
				aligned = pos // 头部半行完整：对齐到下一行行首
			} else {
				line := seg[:len(seg)-1]
				if len(line) > 0 {
					if ev, ok := parseLine(line); ok {
						events = append(events, ev)
					}
				}
			}
			next = pos
		} else if len(seg) > 0 && !droppedHead {
			next = lineStart // 尾部半行：游标留在行首，下轮重读
		}
		first = false
		if rerr != nil {
			if !errors.Is(rerr, io.EOF) {
				err = rerr
			}
			break
		}
	}
	return foldToolResults(events), aligned, next, err
}

// foldToolResults 把 tool_result 并入同页窗口内对应的 tool_use，工具的命令与
// 回报落在同一张卡上。Claude Code 里 tool_result 伪装成 role=user 记录——若单列
// 就会在对话里冒充成「你」说的话（豆包式视图最忌讳这点）。合并后该伪用户行整条
// 删除；跨页窗口、找不到归属工具的 result 保留为独立 result 块由前端再按 tid 兜底。
func foldToolResults(evs []Event) []Event {
	toolByID := make(map[string]*Block)
	for i := range evs {
		for j := range evs[i].Blocks {
			b := &evs[i].Blocks[j]
			if b.Kind == "tool" && b.ToolID != "" {
				toolByID[b.ToolID] = b
			}
		}
	}
	out := make([]Event, 0, len(evs))
	for _, ev := range evs {
		if ev.Role == "user" && len(ev.Blocks) > 0 {
			allResult := true
			for i := range ev.Blocks {
				if ev.Blocks[i].Kind != "result" {
					allResult = false
					break
				}
			}
			if allResult {
				var unmatched []Block
				for _, rb := range ev.Blocks {
					if tb, ok := toolByID[rb.ToolID]; ok && rb.ToolID != "" {
						tb.Result = rb.Text
						tb.ResultErr = rb.IsError
					} else {
						unmatched = append(unmatched, rb) // tool_use 在更早的读取窗口
					}
				}
				if len(unmatched) == 0 {
					continue // 回报全部并入工具卡：不再以「你」的名义单列
				}
				ev.Blocks = unmatched
			}
		}
		out = append(out, ev)
	}
	return out
}

// LastHumanText 返回 transcript 里最近一条真人发言原文（非工具结果/中断/压缩），
// 供「重新生成」取回上一句原话。尾读窗口取大些，避免被超长工具回报挤出视野。
func LastHumanText(path string) (string, bool) {
	f, size, err := openSize(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	start := size - tailBytes*4
	if start < 0 {
		start = 0
	}
	events, _, _, err := readRange(f, start, size, start > 0)
	if err != nil {
		return "", false
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Role != "user" {
			continue
		}
		for _, b := range events[i].Blocks {
			if b.Kind == "text" && strings.TrimSpace(b.Text) != "" {
				return b.Text, true
			}
		}
	}
	return "", false
}

// ---------- jsonl 解析 ----------

type wireRecord struct {
	Type                 string          `json:"type"`
	UUID                 string          `json:"uuid"`
	Timestamp            string          `json:"timestamp"`
	IsSidechain          bool            `json:"isSidechain"`
	InterruptedMessageID string          `json:"interruptedMessageId"` // Ctrl-C：这是一条中断状态记录
	IsCompactSummary     bool            `json:"isCompactSummary"`     // 压缩续接状态记录
	Message              json.RawMessage `json:"message"`
}

type wireMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string 或 []block
}

type wireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"` // tool_result：string 或 []{text}
}

func parseLine(line []byte) (Event, bool) {
	var rec wireRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return Event{}, false
	}
	if rec.Type != "user" && rec.Type != "assistant" || len(rec.Message) == 0 {
		return Event{}, false
	}
	var msg wireMessage
	if err := json.Unmarshal(rec.Message, &msg); err != nil {
		return Event{}, false
	}
	ev := Event{ID: rec.UUID, Role: rec.Type, TS: rec.Timestamp, Sidechain: rec.IsSidechain}
	if ev.ID == "" {
		ev.ID = fmt.Sprintf("%s-%d", rec.Type, len(line)) // 极端兜底，避免前端去重失效
	}

	// 状态记录不是真人发言：Ctrl-C 中断与压缩续接都伪装成 user 记录，
	// 必须在按用户气泡渲染之前截走，变成时间线上的系统分隔行。
	if rec.Type == "user" {
		switch {
		case rec.InterruptedMessageID != "" || isInterruptPlaceholder(msg.Content):
			ev.Role = "system"
			ev.Blocks = []Block{{Kind: "interrupt"}}
			return ev, true
		case rec.IsCompactSummary:
			ev.Role = "system"
			ev.Blocks = []Block{{Kind: "continue"}}
			return ev, true
		}
	}

	// 真人消息的 content 可能直接是字符串
	if len(msg.Content) > 0 && msg.Content[0] == '"' {
		var s string
		if json.Unmarshal(msg.Content, &s) == nil && strings.TrimSpace(s) != "" {
			if IsSystemInjection(s) {
				return Event{}, false // 后台任务通知等系统注入：不是谁说的话，整条丢弃
			}
			ev.Blocks = append(ev.Blocks, Block{Kind: "text", Text: clip(s, maxText)})
		}
		return ev, len(ev.Blocks) > 0
	}

	var blocks []wireBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return Event{}, false
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				if IsSystemInjection(b.Text) {
					continue // 系统注入 text 块：丢弃；若整条只剩它，本行最终不落事件
				}
				ev.Blocks = append(ev.Blocks, Block{Kind: "text", Text: clip(b.Text, maxText)})
			}
		case "thinking":
			if strings.TrimSpace(b.Thinking) != "" {
				ev.Blocks = append(ev.Blocks, Block{
					Kind: "thinking", Text: clip(b.Thinking, maxText), Collapsed: true,
				})
			}
		case "tool_use":
			name, input := b.Name, b.Input
			ev.Blocks = append(ev.Blocks, Block{
				Kind:      "tool",
				ToolID:    b.ID,
				Name:      name,
				Summary:   summarize(name, input),
				Text:      clip(string(input), maxDetail),
				Collapsed: true,
			})
		case "tool_result":
			txt := resultText(b.Content)
			if strings.TrimSpace(txt) == "" && !b.IsError {
				continue
			}
			ev.Blocks = append(ev.Blocks, Block{
				Kind: "result", ToolID: b.ToolUseID, IsError: b.IsError,
				Text: clip(txt, maxDetail), Collapsed: true,
			})
		}
	}
	return ev, len(ev.Blocks) > 0
}

// isInterruptPlaceholder 识别 Ctrl-C 中断占位句（旧记录可能没有
// interruptedMessageId 字段，用占位文本兜底）。content 是字符串或 text 块数组。
func isInterruptPlaceholder(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	check := func(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "[Request interrupted") }
	if raw[0] == '"' {
		var s string
		return json.Unmarshal(raw, &s) == nil && check(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "text" && check(b.Text) {
			return true
		}
	}
	return false
}

func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var out []string
	for _, p := range parts {
		if p.Text != "" {
			out = append(out, p.Text)
		}
	}
	return strings.Join(out, "\n")
}

// summarize 按工具白名单生成一句话动作摘要（不泄露命令全文，全文在 Text 里折叠）。
func summarize(name string, input json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(input, &m) != nil {
		return name
	}
	str := func(k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	base := func(p string) string { return filepath.Base(p) }
	// 摘要刻意纯文本、不含 emoji：图标由模板按工具名叠加（防老板皮相下可整体隐藏）。
	switch name {
	case "Bash", "BashOutput":
		cmd := firstLine(str("command"))
		if cmd == "" {
			cmd = str("command_id")
		}
		return clip("$ "+cmd, maxSummary)
	case "Edit", "MultiEdit":
		return base(str("file_path"))
	case "Write", "NotebookEdit":
		return base(str("file_path"))
	case "Read":
		return base(str("file_path"))
	case "TodoWrite", "TaskUpdate":
		if todos, ok := m["todos"].([]any); ok {
			for _, t := range todos {
				if tm, ok := t.(map[string]any); ok {
					if st, _ := tm["status"].(string); st == "in_progress" {
						return "进度 " + clip(fmt.Sprint(tm["content"]), maxSummary-8)
					}
				}
			}
			return fmt.Sprintf("清单 %d 项", len(todos))
		}
		return "清单更新"
	case "Agent", "Task":
		d := str("description")
		if d == "" {
			d = firstLine(str("prompt"))
		}
		return "子代理 " + clip(d, maxSummary-8)
	case "WebFetch", "WebSearch":
		return clip(str("url")+str("query"), maxSummary-4)
	default:
		return name
	}
}

// Summarize 导出工具入参的一句话动作摘要，供即时桥的实时流复用同一套措辞。
func Summarize(name string, input json.RawMessage) string { return summarize(name, input) }

// ResultText 导出 tool_result 内容的纯文本提取，供实时流复用。
func ResultText(raw json.RawMessage) string { return resultText(raw) }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " …"
}

// ParseTS 解析 transcript 时间戳（UTC RFC3339 带毫秒）。
func ParseTS(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
