// 船队视图模型（v0.7）：一支船队 = 一个 Claude Code 会话。
// 船队状态直接复用官方发现口（claude agents）原生三态，思港不另造：
//   - busy    → 工作中
//   - waiting → 待你输入（等权限确认/等补充信息）
//   - idle    → 已停止（进程还在，随时能再说话）
//   - 离线    → 已停止（进程不在，可回看可唤醒）
//
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
	case "restarting":
		return fleetState{Key: "restarting", Text: "重连中"}
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

// fleetDefaultName 给任务派生默认船名：挂航线用「短码-需求」，否则用任务标题。
func (s *Server) fleetDefaultName(rec *store.Record) string {
	title := strings.TrimSpace(rec.Meta.Title)
	if rec.Meta.Route != "" {
		if rt, err := s.V.GetRoute(rec.Meta.Route); err == nil {
			return rt.Meta.Code + "-" + clipRunes(title, maxFleetNameRunes-utf8RuneLen(rt.Meta.Code)-1)
		}
	}
	return clipRunes(title, maxFleetNameRunes)
}

func utf8RuneLen(s string) int { return len([]rune(s)) }

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// handleTaskLaunch 在指定项目目录新开一支桥船队，把任务挂上去。
// 前端用 fetch 提交：成功返 JSON 跳转船台，失败错误就地显示（不再踢回首页）。
func (s *Server) handleTaskLaunch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	dir := filepath.Clean(strings.TrimSpace(r.PostFormValue("dir")))
	if !filepath.IsAbs(dir) {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "要给项目目录的绝对路径，船队才知道在哪儿开工"})
		return
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "本机找不到这个目录：" + dir})
		return
	}
	rec, err := s.V.GetTask(id)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	msg := strings.TrimSpace(r.PostFormValue("msg"))
	if msg == "" {
		msg = launchPrompt(rec)
	}
	if len([]rune(msg)) > 8000 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "开场白太长（8000 字以内）"})
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		name = s.fleetDefaultName(rec)
	}
	if len([]rune(name)) > maxFleetNameRunes || strings.ContainsAny(name, "\n\r") {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "船名太长或含换行"})
		return
	}
	// 航线可在 launch 表单里现挂：校验通过后先落到任务上，再随船队登记。
	routeID := strings.TrimSpace(r.PostFormValue("route"))
	if routeID != "" {
		if _, err := s.V.GetRoute(routeID); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if routeID != rec.Meta.Route {
			if err := s.V.SetTaskRoute(id, routeID); err != nil {
				writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
	} else {
		routeID = rec.Meta.Route
	}

	sid, _, err := s.bridge.Start(dir, "", name, msg)
	if err != nil || !looksLikeSID(sid) {
		detail := "新开船队失败"
		if err != nil {
			detail += "：" + err.Error()
		}
		writeJSONStatus(w, http.StatusBadGateway, map[string]string{"error": detail})
		return
	}
	if err := s.V.SaveFleet(store.FleetInfo{
		SID: sid, Name: name, Dir: dir, Task: id, Route: routeID,
	}); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "船队已开（" + shortSid(sid) + "）但名册登记失败：" + err.Error()})
		return
	}
	if _, err := s.V.BindSession(id, sid, dir, dispatcher); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "船队已开但挂载失败：" + err.Error()})
		return
	}
	// 立即拉一次发现口，新船队秒进列表（不必等 30s 轮询）。
	if s.poller != nil {
		go s.poller.poll()
	}
	writeJSON(w, map[string]string{
		"ok":  "1",
		"sid": sid,
		"url": "/sessions/" + sid + "?dir=" + url.QueryEscape(dir),
	})
}
