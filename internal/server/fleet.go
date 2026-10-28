// 船队视图模型（v0.7）：一支船队 = 一个 Claude Code 会话。
// 船队状态直接复用官方发现口（claude agents）原生三态，思港不另造：
//   - busy    → 工作中
//   - waiting → 待你输入（等权限确认/等补充信息）
//   - idle    → 已停止（进程还在，随时能再说话）
//   - 离线    → 已停止（进程不在，可回看可唤醒）
// 结局标记（人给的）覆盖状态文案：圆满结束 / 中途停止。
package server

import (
	"fmt"
	"net/http"
	"net/url"
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
	if info, ok := s.resolveSession(sid, r.URL.Query().Get("dir")); ok && info.anon {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "船队还没说过话，先没有结局可标记"})
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
	if isAnonSID(sid) {
		redirectErr(w, r, "这支船队还没说过话，挂不住——等它在自己终端开口，或直接新开一支")
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

// ---------- 新开船队跑任务 ----------

// launchPrompt 是新船队的默认开场白：任务单里最有用的信息一次带上，
// 新会话一开口就知道要干什么。用户可在表单里改。
func launchPrompt(rec *store.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "思港挂来一个想法（%s）：%s", rec.Meta.ID, rec.Meta.Title)
	if rec.Meta.Goal != "" {
		b.WriteString("\n服务的目标：" + rec.Meta.Goal)
	}
	if body := strings.TrimSpace(rec.Body); body != "" {
		b.WriteString("\n\n" + body)
	}
	b.WriteString("\n\n先看清这个想法、给出你的理解和打算，需要我确认的地方先问我。")
	if r := []rune(b.String()); len(r) > 6000 {
		return string(r[:6000])
	}
	return b.String()
}

// handleTaskLaunch 在指定项目目录新开一支后台船队（claude --bg），
// 把任务挂上去，然后跳去新船队的船台看它开工。
func (s *Server) handleTaskLaunch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	dir := filepath.Clean(strings.TrimSpace(r.PostFormValue("dir")))
	if !filepath.IsAbs(dir) {
		redirectErr(w, r, "要给项目目录的绝对路径，船队才知道在哪儿开工")
		return
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		redirectErr(w, r, "本机找不到这个目录："+dir)
		return
	}
	rec, err := s.V.GetTask(id)
	if err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	msg := strings.TrimSpace(r.PostFormValue("msg"))
	if msg == "" {
		msg = launchPrompt(rec)
	}
	if len([]rune(msg)) > 8000 {
		redirectErr(w, r, "开场白太长（8000 字以内）")
		return
	}

	sid, err := s.runBackground(dir, "", msg)
	if err != nil || !looksLikeSID(sid) {
		detail := "新开船队失败"
		if err != nil {
			detail += "：" + err.Error()
		}
		redirectErr(w, r, detail)
		return
	}
	home, _ := os.UserHomeDir()
	waitTranscript(home, dir, sid, 8*time.Second)
	if _, err := s.V.BindSession(id, sid, dir, dispatcher); err != nil {
		redirectErr(w, r, "船队已开（"+shortSid(sid)+"）但挂载失败："+err.Error())
		return
	}
	// 立即拉一次发现口，新船队秒进列表（不必等 30s 轮询）。
	if s.poller != nil {
		go s.poller.poll()
	}
	http.Redirect(w, r,
		"/sessions/"+sid+"?dir="+url.QueryEscape(dir), http.StatusSeeOther)
}
