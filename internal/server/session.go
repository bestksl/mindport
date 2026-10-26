// 会话船台（ADR-0008）：把单个 Claude Code 会话的 transcript jsonl 桥接成
// 幂等、可全量滚动的实时时间线，并支持调度长继续输入——
// 会话空闲时 `claude -p --resume <sid> --bg` 续跑，正忙时留言进信箱等钩子注入。
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bestksl/mindport/internal/store"
	"github.com/bestksl/mindport/internal/transcript"
)

// resumeTimeout：--bg 是「排队即返回」，正常秒回；30s 只兜 CLI 卡死。
const resumeTimeout = 30 * time.Second

// sessInfo 是定位一个会话所需的全部信息（官方快照 + 磁盘心跳合并）。
type sessInfo struct {
	dir    string
	name   string
	online bool
	status string // busy | waiting | idle | ""（离线）
	state  string
}

// resolveSession 在官方发现口快照与磁盘心跳里找会话。
// 离线会话（快照缺席、心跳还在）也找得到——transcript 历史照读，还能 --resume 唤醒。
func (s *Server) resolveSession(sid string) (sessInfo, bool) {
	var info sessInfo
	found := false
	if s.poller != nil {
		if live, ok := s.poller.snapshot()[sid]; ok {
			info = sessInfo{
				dir: live.CWD, name: live.Name, online: true,
				status: live.Status, state: live.State,
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

	Found  bool // 心跳/快照里有此会话
	HasLog bool // transcript 文件已落盘

	Events  []transcript.Event
	Cursor  int64
	From    int64
	Size    int64
	Pending int
	Error   string
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	d := sessionPage{Agents: s.listPresence(), SID: sid, ShortSID: shortSid(sid)}
	if !sidOK(sid) {
		render(w, http.StatusNotFound, sessionTmpl, d)
		return
	}
	info, found := s.resolveSession(sid)
	d.Found = found
	d.Name = info.name
	d.Dir = info.dir
	d.Short = filepath.Base(info.dir)
	d.Online = info.online
	d.Status = info.status
	d.State = info.state
	d.Busy = info.status == "busy" || info.status == "waiting"
	d.Pending, _ = s.V.MailPending(sid)

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
	Events  []transcript.Event `json:"events"`
	Cursor  int64              `json:"cursor"`
	Size    int64              `json:"size"`
	Online  bool               `json:"online"`
	Busy    bool               `json:"busy"`
	Status  string             `json:"status"`
	State   string             `json:"state"`
	Pending int                `json:"pending"`
}

func (s *Server) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	info, found := s.resolveSession(sid)
	if !found || info.dir == "" {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	resp := streamResp{
		Online: info.online,
		Busy:   info.status == "busy" || info.status == "waiting",
		Status: info.status,
		State:  info.state,
		Events: []transcript.Event{},
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
	info, found := s.resolveSession(sid)
	if !found || info.dir == "" {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
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
	info, found := s.resolveSession(sid)
	if !found || info.dir == "" {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}

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
	writeJSON(w, sayResp{Delivered: "resume", Detail: "已唤醒该会话，船在后台接着跑"})
}

func (s *Server) queueMail(w http.ResponseWriter, sid, msg string) {
	if _, err := s.V.AppendMail(sid, msg); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "留言入箱失败: " + err.Error()})
		return
	}
	n, _ := s.V.MailPending(sid)
	writeJSON(w, sayResp{Delivered: "mail", Pending: n, Detail: "留言已进信箱，船下一轮开工时自动收到"})
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
	exe := s.claudeExe()
	if exe == "" {
		return os.ErrNotExist
	}
	cctx, cancel := context.WithTimeout(context.Background(), resumeTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, exe, "-p", msg, "--resume", sid, "--bg")
	cmd.Dir = info.dir
	// transcript 与鉴权由外层保证；继承 serve 环境即可（HOME/PATH 都是本机用户的）。
	return cmd.Run()
}

func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
