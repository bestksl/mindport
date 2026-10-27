// 船队视图模型（v0.7）：一支船队 = 一个 Claude Code 会话。
// 船队状态直接复用官方发现口（claude agents）原生三态，思港不另造：
//   - busy    → 工作中
//   - waiting → 待你输入（等权限确认/等补充信息）
//   - idle    → 已停止（进程还在，随时能再说话）
//   - 离线    → 已停止（进程不在，可回看可唤醒）
// 结局标记（人给的）覆盖状态文案：圆满结束 / 中途停止。
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bestksl/mindport/internal/store"
	"github.com/bestksl/mindport/internal/transcript"
)

const (
	stateWorking = "working"
	stateWaiting = "waiting"
	stateIdle    = "idle"
	stateOffline = "offline"
)

type fleetState struct {
	Key  string
	Text string
}

func fleetStateOf(online bool, status string) fleetState {
	if !online {
		return fleetState{Key: stateOffline, Text: "已停止"}
	}
	switch status {
	case "busy":
		return fleetState{Key: stateWorking, Text: "工作中"}
	case "waiting":
		return fleetState{Key: stateWaiting, Text: "待你输入"}
	default:
		return fleetState{Key: stateIdle, Text: "已停止"}
	}
}

// markText 给结局标记的文案。
func markText(outcome string) string {
	switch outcome {
	case store.MarkGood:
		return "✅ 圆满结束"
	case store.MarkStop:
		return "⏹ 中途停止"
	}
	return ""
}

// fleetOption 是任务页「挂到哪支船队」下拉里的一支船队。
type fleetOption struct {
	SID       string
	Dir       string
	Proj      string // 目录 basename
	Title     string
	Short     string
	Online    bool
	Status    string
	StateText string
}

// fleetOptions 返回最近活跃船队 + 当前在线船队，供任务页挂船队选择。
// boundSID 非空时确保它排在第一位（哪怕已掉出最近列表）。
func (s *Server) fleetOptions(boundSID, boundDir, boundTitle string) []fleetOption {
	home, _ := os.UserHomeDir()
	metas, _ := transcript.RecentSessions(home, recentScanFiles, recentTotal)
	live := map[string]cliSession{}
	if s.poller != nil {
		live = s.poller.snapshot()
	}
	out := make([]fleetOption, 0, len(metas)+1)
	seen := map[string]bool{}
	add := func(sid, dir, title string) {
		if sid == "" || seen[sid] {
			return
		}
		seen[sid] = true
		op := fleetOption{SID: sid, Dir: dir, Proj: filepath.Base(dir), Title: title, Short: shortSid(sid)}
		if ls, ok := live[sid]; ok {
			op.Online = true
			op.Status = ls.Status
		}
		op.StateText = fleetStateOf(op.Online, op.Status).Text
		out = append(out, op)
	}
	if boundSID != "" {
		add(boundSID, boundDir, boundTitle)
	}
	for _, m := range metas {
		add(m.SID, m.Dir, m.Title)
	}
	return out
}

// fleetOptionTitle 从任务页已知信息尽量还原船队显示名。
func boundFleetTitle(sid, dir, fallback string) string {
	if fallback != "" {
		return fallback
	}
	return shortSid(sid)
}

// ---------- 结局标记 ----------

func (s *Server) handleSessionMark(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	_ = r.ParseForm()
	outcome := strings.TrimSpace(r.PostFormValue("outcome"))
	if !store.ValidMarkOutcome(outcome) {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "结局标记非法"})
		return
	}
	dir := strings.TrimSpace(r.PostFormValue("dir"))
	if dir == "" {
		if info, ok := s.resolveSession(sid, r.URL.Query().Get("dir")); ok && info.dir != "" {
			dir = info.dir
		}
	}
	if err := s.V.MarkSession(sid, dir, outcome); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"outcome": outcome, "text": markText(outcome), "at": time.Now().Format("01-02 15:04")})
}

func (s *Server) handleSessionUnmark(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	if err := s.V.UnmarkSession(sid); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// ---------- 任务挂船队 ----------

func (s *Server) handleTaskBind(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	// 表单值 sid|dir，与派船下拉同构；dir 可空（历史会话建议带上）。
	sid, dir := splitTarget(r.PostFormValue("fleet"))
	sid, dir = strings.TrimSpace(sid), strings.TrimSpace(dir)
	if !sidOK(sid) {
		redirectErr(w, r, "要选一支船队（会话）再挂")
		return
	}
	if _, err := s.V.BindSession(id, sid, dir, dispatcher); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
}

func (s *Server) handleTaskUnbind(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.V.UnbindSession(id, dispatcher); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
}
