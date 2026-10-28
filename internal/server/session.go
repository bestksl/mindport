// 会话船台（ADR-0008）：把单个 Claude Code 会话的 transcript jsonl 桥接成
// 幂等、可全量滚动的实时时间线，并支持调度长继续输入——
// 会话空闲时 `claude -p --resume <sid> --bg` 续跑，正忙时留言进信箱等钩子注入。
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bestksl/mindport/internal/store"
	"github.com/bestksl/mindport/internal/transcript"
)

// resumeTimeout：--bg 是「排队即返回」，正常秒回；30s 只兜 CLI 卡死。
const resumeTimeout = 30 * time.Second

// sessInfo 是定位一个会话所需的全部信息（官方快照 + 磁盘心跳合并）。
type sessInfo struct {
	dir        string
	name       string
	online     bool
	status     string // busy | waiting | idle | ""（离线）
	state      string
	waitingFor string
	anon       bool // 进程在跑但还没说话：无 transcript、sid 是 proc-<pid>
}

// resolveSession 在官方发现口快照与磁盘心跳里找会话；dirHint（URL ?dir=）
// 是「扫盘模式」兜底——历史会话从不上心跳也定位得到，transcript 照读、可 --resume。
// 离线会话（快照缺席、心跳还在）也找得到。
func (s *Server) resolveSession(sid, dirHint string) (sessInfo, bool) {
	var info sessInfo
	found := false
	if s.poller != nil {
		if live, ok := s.poller.snapshot()[sid]; ok {
			info = sessInfo{
				dir: live.CWD, name: live.Name, online: true,
				status: live.Status, state: live.State, waitingFor: live.WaitingFor,
				anon:   live.Anon,
			}
			found = true
		}
	}
	for _, p := range s.listPresence() {
		if p.SessionID != sid {
			continue
		}
		found = true
		if info.dir == "" {
			info.dir = p.Dir
		}
		if info.name == "" {
			info.name = p.Name
		}
		if !info.online {
			// 快照缺席：listPresence 已据此置灰
			info.online = p.Online
		}
	}
	// 心跳/发现口都没有：相信 URL 带来的目录提示，但必须自证——
	// 绝对路径且对应 transcript 文件真实存在（防目录穿越与瞎编）。
	if info.dir == "" && dirHint != "" && filepath.IsAbs(dirHint) {
		if home, err := os.UserHomeDir(); err == nil && transcript.Exists(home, dirHint, sid) {
			info.dir = dirHint
			found = true
		}
	}
	return info, found
}

// shortSid 是页面展示用的短 id（UUID 前 8 位）。
func shortSid(sid string) string {
	if len(sid) > 8 {
		return sid[:8]
	}
	return sid
}

// sidOK 只收 UUID 形态的 sid：它要拼本地路径，URL 来的参数必须先自证。
func sidOK(sid string) bool {
	if sid == "" || len(sid) > 64 {
		return false
	}
	for _, r := range sid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

func (s *Server) transcriptPath(info sessInfo, sid string) (string, bool) {
	if info.dir == "" {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	p := transcript.Path(home, info.dir, sid)
	if _, err := os.Stat(p); err != nil {
		return p, false
	}
	return p, true
}

// ---------- 页面 ----------

type sessionPage struct {
	Agents   []store.Presence // 顶栏船队
	SID      string
	ShortSID string
	Name     string
	Dir      string
	Short    string // 目录 basename
	Online   bool
	Busy     bool
	Status   string
	State    string

	StateKey    string // working | waiting | idle | offline
	StateText   string
	WaitingFor  string
	HasMark     bool
	Mark        store.SessionMark
	MarkText    string
	BoundTasks  []store.Record // 挂在这支船队上的集装箱

	Found    bool // 心跳/快照/目录提示里有此会话
	HasLog   bool // transcript 文件已落盘
	Anon     bool // 进程在跑但还没说话：只显示空态，船台不可交互
	AnonPID  string // Anon 时展示进程 pid（sid 是 proc-<pid> 临时键）
	Auth     bool // 顶栏据此显示登出口
	FromList bool // 从 /sessions?dir= 列表点进来：返回键回目录列表

	Events  []transcript.Event
	Cursor  int64
	From    int64
	Size    int64
	Pending int
	Error   string
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	d := sessionPage{Agents: s.listPresence(), SID: sid, ShortSID: shortSid(sid), Auth: s.authOn()}
	if !sidOK(sid) {
		render(w, http.StatusNotFound, sessionTmpl, d)
		return
	}
	dirHint := r.URL.Query().Get("dir")
	info, found := s.resolveSession(sid, dirHint)
	d.Found = found
	d.FromList = dirHint != ""
	d.Name = info.name
	d.Dir = info.dir
	d.Short = filepath.Base(info.dir)
	d.Online = info.online
	d.Status = info.status
	d.State = info.state
	d.WaitingFor = info.waitingFor
	d.Busy = info.status == "busy" || info.status == "waiting"
	fs := fleetStateOf(info.online, info.status)
	d.StateKey, d.StateText = fs.Key, fs.Text
	if mk, ok := s.V.SessionMark(sid); ok {
		d.HasMark, d.Mark, d.MarkText = true, mk, markText(mk.Outcome)
	}
	d.Pending, _ = s.V.MailPending(sid)
	d.BoundTasks, _ = s.V.TasksBySession(sid)
	// 历史会话没有心跳简介：用 transcript 首条真人发言当船台标题
	if found && d.Name == "" && info.dir != "" {
		if home, err := os.UserHomeDir(); err == nil {
			d.Name = transcript.Meta(home, info.dir, sid).Title
		}
	}

	if found && info.anon {
		d.Anon = true
		d.AnonPID = strings.TrimPrefix(sid, "proc-")
		if d.Name == "" {
			d.Name = "新开的会话（还没说话）"
		}
		render(w, http.StatusOK, sessionTmpl, d)
		return
	}
	if found {
		if p, ok := s.transcriptPath(info, sid); ok {
			d.HasLog = true
			if pg, err := transcript.ReadTail(p); err == nil {
				d.Events = pg.Events
				d.Cursor = pg.Cursor
				d.From = pg.From
				d.Size = pg.Size
			}
		}
	}
	render(w, http.StatusOK, sessionTmpl, d)
}

// ---------- 增量 / 上翻 JSON ----------

type streamResp struct {
	Events     []transcript.Event `json:"events"`
	Cursor     int64              `json:"cursor"`
	Size       int64              `json:"size"`
	Online     bool               `json:"online"`
	Busy       bool               `json:"busy"`
	Status     string             `json:"status"`
	State      string             `json:"state"`
	WaitingFor string             `json:"waiting_for,omitempty"`
	Mark       string             `json:"mark,omitempty"` // good | stop（人给的结局，覆盖状态文案）
	Pending    int                `json:"pending"`
}

func (s *Server) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	info, found := s.resolveSession(sid, r.URL.Query().Get("dir"))
	if !found || info.dir == "" {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	if info.anon {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "这支船队还没说过话，没有船台"})
		return
	}
	resp := streamResp{
		Online:     info.online,
		Busy:       info.status == "busy" || info.status == "waiting",
		Status:     info.status,
		State:      info.state,
		WaitingFor: info.waitingFor,
		Events:     []transcript.Event{},
	}
	if mk, ok := s.V.SessionMark(sid); ok {
		resp.Mark = mk.Outcome
	}
	resp.Pending, _ = s.V.MailPending(sid)
	home, _ := os.UserHomeDir()
	p := transcript.Path(home, info.dir, sid)
	from := int64(atoiDefault(r.URL.Query().Get("from"), 0))
	if pg, err := transcript.ReadSince(p, from); err == nil {
		if len(pg.Events) > 0 { // nil 切片会编码成 null，前端要稳定的 []
			resp.Events = pg.Events
		}
		resp.Cursor = pg.Cursor
		resp.Size = pg.Size
	} else {
		resp.Cursor = from
	}
	writeJSON(w, resp)
}

type historyResp struct {
	Events []transcript.Event `json:"events"`
	From   int64              `json:"from"`
	Size   int64              `json:"size"`
}

func (s *Server) handleSessionHistory(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	info, found := s.resolveSession(sid, r.URL.Query().Get("dir"))
	if !found || info.dir == "" {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	if info.anon {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "这支船队还没说过话，没有往事可翻"})
		return
	}
	home, _ := os.UserHomeDir()
	p := transcript.Path(home, info.dir, sid)
	before := int64(atoiDefault(r.URL.Query().Get("before"), 0))
	pg, err := transcript.ReadBefore(p, before)
	if err != nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "transcript 读不到"})
		return
	}
	evs := pg.Events
	if evs == nil {
		evs = []transcript.Event{}
	}
	writeJSON(w, historyResp{Events: evs, From: pg.From, Size: pg.Size})
}

// ---------- 继续输入 ----------

type sayResp struct {
	Delivered string `json:"delivered"` // resume=已唤醒续跑；mail=排队留言
	Pending   int    `json:"pending,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

func (s *Server) handleSessionSay(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	_ = r.ParseForm()
	msg := strings.TrimSpace(r.PostFormValue("msg"))
	if msg == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "要说的话不能为空"})
		return
	}
	info, found := s.resolveSession(sid, r.URL.Query().Get("dir"))
	if !found || info.dir == "" {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	if info.anon {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "这支船队还没说过话；请在它自己的终端里开口，或从任务上新开一支"})
		return
	}
	// 又开口说话 = 结局翻篇：旧的「圆满结束/中途停止」标记自动撤掉。
	_ = s.V.UnmarkSession(sid)

	// 正忙（busy/waiting）：官方拒绝并发 resume，直接进信箱，下轮提交时钩子注入。
	if info.online && info.status != "idle" {
		s.queueMail(w, sid, msg)
		return
	}
	// 空闲或离线：同会话后台续跑；与官方并发限制撞车（竞态）时兜底进信箱，不丢指令。
	if err := s.resumeSession(info, sid, msg); err != nil {
		s.queueMail(w, sid, msg)
		return
	}
	writeJSON(w, sayResp{Delivered: "resume", Detail: "已唤醒会话，正在后台接着跑"})
}

func (s *Server) queueMail(w http.ResponseWriter, sid, msg string) {
	if _, err := s.V.AppendMail(sid, msg); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "留言入箱失败: " + err.Error()})
		return
	}
	n, _ := s.V.MailPending(sid)
	writeJSON(w, sayResp{Delivered: "mail", Pending: n, Detail: "留言已排队，船队下一轮开工时自动收到"})
}

// claudeExe 找官方 CLI：优先复用 poller 已定位的路径，再走 PATH 与常见安装位。
func (s *Server) claudeExe() string {
	if s.poller != nil && s.poller.exe != "" {
		return s.poller.exe
	}
	if exe, err := exec.LookPath("claude"); err == nil {
		return exe
	}
	if home, err := os.UserHomeDir(); err == nil {
		cand := filepath.Join(home, ".local", "bin", "claude")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return ""
}

func (s *Server) resumeSession(info sessInfo, sid, msg string) error {
	_, err := s.runBackground(info.dir, sid, msg)
	return err
}

func looksLikeSID(v string) bool {
	if len(v) != 36 || v[8] != '-' || v[13] != '-' {
		return false
	}
	return sidOK(v)
}

// extractSessionID 递归找输出里任何「session 身份」键的 UUID 值。
func extractSessionID(b []byte) string {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	found := ""
	var walk func(any)
	walk = func(x any) {
		if found != "" {
			return
		}
		switch t := x.(type) {
		case map[string]any:
			for k, vv := range t {
				lk := strings.ToLower(strings.ReplaceAll(k, "_", ""))
				if s, ok := vv.(string); ok && (lk == "sessionid" || lk == "session") && looksLikeSID(s) {
					found = s
					return
				}
			}
			for _, vv := range t {
				walk(vv)
			}
		case []any:
			for _, it := range t {
				walk(it)
			}
		}
	}
	walk(v)
	return found
}

// bgAckRe 匹配 2.1.310 真人横幅 "backgrounded · c8e5a536"——该版本即便带
// --output-format json，--bg 仍只回这条人类横幅，短 id 是唯一抓手。
var bgAckRe = regexp.MustCompile(`backgrounded[^\n]*?\b([0-9a-f]{8})\b`)

// runBackground 用 2.1.x 语法调起一个后台会话（`claude --bg [--resume sid]
// --output-format json <prompt>`；旧语法 `-p --bg` 已被 CLI 明确拒绝）。
// resumeSID 为空=新开船队，返回值是新会话 sid；非空=续话，返回值即原 sid。
func (s *Server) runBackground(dir, resumeSID, msg string) (string, error) {
	exe := s.claudeExe()
	if exe == "" {
		return "", os.ErrNotExist
	}
	args := []string{"--bg", "--output-format", "json"}
	if resumeSID != "" {
		args = append(args, "--resume", resumeSID)
	}
	args = append(args, msg)
	cctx, cancel := context.WithTimeout(context.Background(), resumeTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, exe, args...)
	cmd.Dir = dir
	// transcript 与鉴权由外层保证；继承 serve 环境即可（HOME/PATH 都是本机用户的）。
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if ee, ok := err.(*exec.ExitError); ok {
			if s := strings.TrimSpace(string(ee.Stderr)); s != "" {
				detail = s
			}
		}
		if detail != "" {
			return resumeSID, fmt.Errorf("claude 没接活：%s", clipText(detail, 300))
		}
		return resumeSID, err
	}
	if resumeSID != "" {
		return resumeSID, nil
	}
	// 新版若老老实实给 JSON 最好；2.1.310 实测只给 "backgrounded · <短id>"，
	// 拿短 id 去发现口反查完整 sessionId（UUID 才能拼 transcript 路径与挂载）。
	if sid := extractSessionID(out); sid != "" {
		return sid, nil
	}
	m := bgAckRe.FindSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("CLI 已排队但没返回会话 id（输出：%s）", clipText(strings.TrimSpace(string(out)), 200))
	}
	short := string(m[1])
	if sid := waitAgentSID(exe, short, 8*time.Second); sid != "" {
		return sid, nil
	}
	return "", fmt.Errorf("CLI 已排队（短 id %s），但发现口暂时没报出完整会话 id，稍等可从船队列表找到它再挂任务", short)
}

// waitAgentSID 用短 id 轮询 `claude agents --json` 反查完整 sessionId：
// 后台会话出生到进发现口有秒级延迟。
func waitAgentSID(exe, short string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		b, err := exec.CommandContext(cctx, exe, "agents", "--json").Output()
		cancel()
		if err == nil {
			if ss, perr := parseAgents(b); perr == nil {
				for _, a := range ss {
					if a.ID == short && looksLikeSID(a.SessionID) {
						return a.SessionID
					}
				}
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return ""
}

// clipText 给塞进错误/提示的 CLI 原文限长。
func clipText(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// waitTranscript 等新会话 transcript 落盘（--bg 排队即返回，文件可能稍后才生）。
func waitTranscript(home, dir, sid string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if transcript.Exists(home, dir, sid) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return transcript.Exists(home, dir, sid)
}

func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
