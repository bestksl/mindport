// Package server 用标准库 net/http 把 vault 暴露为移动端优先的 Web 看板
// （手机浏览器/PWA）与 JSON API。静态资源经 go:embed 打进同一二进制。
// 文件变化（CLI/agent/手改/网页写）由 fsnotify 监听，经 SSE 实时推给页面。
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yuin/goldmark"

	"github.com/bestksl/mindport/internal/store"
)

//go:embed static/*
var staticFS embed.FS

const actorCookie = "mp_actor"

// Server 持有 vault 与事件广播器。
type Server struct {
	V      *store.Vault
	broker *broker
}

// New 构造 Server 并启动 vault 文件监听（监听失败不影响主流程）。
func New(v *store.Vault) *Server {
	s := &Server{V: v, broker: newBroker()}
	go s.watchVault()
	return s
}

// ---------- SSE 广播 ----------

type broker struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func newBroker() *broker { return &broker{clients: map[chan struct{}]struct{}{}} }

func (b *broker) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *broker) unsubscribe(ch chan struct{}) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func (b *broker) publish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- struct{}{}:
		default: // 客户端还没取走上一条，丢弃中间态即可
		}
	}
}

// watchVault 监听 tasks/ 与 inbox/，把文件事件去抖后广播。
func (s *Server) watchVault() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer w.Close()
	for _, d := range []string{"tasks", "inbox"} {
		_ = w.Add(filepath.Join(s.V.Root, d))
	}
	var debounce *time.Timer
	for {
		select {
		case _, ok := <-w.Events:
			if !ok {
				return
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(250*time.Millisecond, s.broker.publish)
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// ---------- 路由 ----------

// Routes 构造路由器（Go 1.22+ 方法与路径变量）。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /identity", s.handleIdentity)
	mux.HandleFunc("POST /thoughts", s.handleCreateThought)
	mux.HandleFunc("POST /thoughts/promote", s.handlePromoteThought)
	mux.HandleFunc("POST /tasks", s.handleCreateTask)
	mux.HandleFunc("GET /tasks/{id}", s.handleTaskDetail)
	mux.HandleFunc("POST /tasks/{id}/publish", s.handlePublish)
	mux.HandleFunc("POST /tasks/{id}/claim", s.handleClaim)
	mux.HandleFunc("POST /tasks/{id}/done", s.handleDone)
	mux.HandleFunc("POST /tasks/{id}/block", s.handleBlock)
	mux.HandleFunc("POST /tasks/{id}/unblock", s.handleUnblock)
	mux.HandleFunc("POST /tasks/{id}/log", s.handleLog)
	mux.HandleFunc("POST /tasks/{id}/check", s.handleCheck)
	mux.HandleFunc("POST /tasks/{id}/edit", s.handleEdit)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("GET /api/tasks", s.handleAPITasks)
	mux.HandleFunc("GET /api/next", s.handleAPINext)
	mux.HandleFunc("GET /api/agents", s.handleAPIAgents)

	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /manifest.webmanifest", serveStatic("static/manifest.webmanifest", "application/manifest+json"))
	mux.HandleFunc("GET /sw.js", serveStatic("static/sw.js", "application/javascript"))
	mux.HandleFunc("GET /icon.svg", serveStatic("static/icon.svg", "image/svg+xml"))

	return mux
}

// ---------- 视图模型 ----------

type boardData struct {
	Actor    string
	IsTask   bool
	BackID   string
	Thoughts []store.Record
	Drafts   []store.Record
	Cols     []column
	Agents   []store.Presence
	ByTask   map[string]store.Presence // 在线执行者 → 正在做的任务
	Now      time.Time
	Error    string
}

type column struct {
	Key   string
	Title string
	Tasks []store.Record
}

type acceptItem struct {
	N       int
	Checked bool
	HTML    template.HTML
}

type taskView struct {
	R               *store.Record
	Actor           string
	IsTask          bool
	BackID          string
	Agents          []store.Presence
	Agent           *store.Presence // 归属执行者的实时心跳（可能为 nil）
	DescriptionHTML template.HTML
	Accept          []acceptItem
	AcceptDone      int
	Logs            []string
	Extra           template.HTML
}

// ---------- 页面处理器 ----------

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	thoughts, _ := s.V.ListThoughts()
	all, _ := s.V.ListTasks("")
	agents, byTask := s.presenceIndex()
	d := boardData{
		Actor: actor(r), Thoughts: thoughts, Now: time.Now(),
		Agents: agents, ByTask: byTask,
		Error: r.URL.Query().Get("error"),
		Cols: []column{
			{Key: "ready", Title: "⚓ 待领取"},
			{Key: "busy", Title: "🚢 进行中"},
			{Key: "done", Title: "🏁 已归港"},
		},
	}
	for _, t := range all {
		switch t.Meta.Status {
		case store.StatusDraft:
			d.Drafts = append(d.Drafts, t)
		case store.StatusReady:
			d.Cols[0].Tasks = append(d.Cols[0].Tasks, t)
		case store.StatusInProgress, store.StatusBlocked:
			d.Cols[1].Tasks = append(d.Cols[1].Tasks, t)
		case store.StatusDone:
			if len(d.Cols[2].Tasks) < 12 {
				d.Cols[2].Tasks = append(d.Cols[2].Tasks, t)
			}
		}
	}
	render(w, http.StatusOK, boardTmpl, d)
}

func (s *Server) handleTaskDetail(w http.ResponseWriter, r *http.Request) {
	rec, err := s.V.GetTask(r.PathValue("id"))
	if err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	v := buildTaskView(rec, actor(r))
	v.IsTask = true
	v.BackID = rec.Meta.ID
	agents, _ := s.presenceIndex()
	v.Agents = agents
	if a := rec.Meta.AssigneeValue(); a != "" {
		for i := range agents {
			if agents[i].Actor == a {
				v.Agent = &agents[i]
				break
			}
		}
	}
	render(w, http.StatusOK, taskTmpl, v)
}

func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	http.SetCookie(w, &http.Cookie{
		Name: actorCookie, Value: r.PostFormValue("actor"),
		Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode,
	})
	back := r.PostFormValue("back")
	if back == "" {
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (s *Server) handleCreateThought(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if _, err := s.V.AddThought(r.PostFormValue("title"), "", "mobile"); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handlePromoteThought(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	rec, err := s.V.PromoteThought(r.PostFormValue("name"), r.PostFormValue("importance"))
	if err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/tasks/"+rec.Meta.ID, http.StatusSeeOther)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	imp := r.PostFormValue("importance")
	if imp == "" {
		imp = store.ImpNormal
	}
	// 网页默认进草稿打磨；勾选「直接发布」才立即出港。
	draft := r.PostFormValue("publish") != "1"
	rec, err := s.V.NewTask(r.PostFormValue("title"), imp, "", draft)
	if err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/tasks/"+rec.Meta.ID, http.StatusSeeOther)
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id := r.PathValue("id")
	if _, err := s.V.Publish(id, actor(r), r.PostFormValue("assignee")); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	if r.PostFormValue("from") == "task" {
		http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// presenceIndex 返回全部心跳，以及「在线且正在做某任务」的 task→Presence 索引。
func (s *Server) presenceIndex() ([]store.Presence, map[string]store.Presence) {
	agents, _ := s.V.ListPresence()
	byTask := map[string]store.Presence{}
	for _, p := range agents {
		if p.Online && p.Task != "" {
			byTask[p.Task] = p
		}
	}
	return agents, byTask
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	if _, err := s.V.Claim(r.PathValue("id"), actor(r)); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	backToBoardOrTask(w, r)
}

func (s *Server) handleDone(w http.ResponseWriter, r *http.Request) {
	if _, err := s.V.Done(r.PathValue("id"), actor(r)); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if _, err := s.V.Block(r.PathValue("id"), r.PostFormValue("reason"), actor(r)); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	backToBoardOrTask(w, r)
}

func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	if _, err := s.V.Unblock(r.PathValue("id"), actor(r)); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	backToBoardOrTask(w, r)
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id := r.PathValue("id")
	if _, err := s.V.Log(id, r.PostFormValue("line"), actor(r)); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id := r.PathValue("id")
	n := atoiDefault(r.PostFormValue("n"), 0)
	checked := r.PostFormValue("checked") == "1"
	if _, _, _, err := s.V.CheckItem(id, n, checked, actor(r)); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id := r.PathValue("id")
	rec, err := s.V.UpdateTask(
		id,
		r.PostFormValue("title"),
		r.PostFormValue("body"),
		r.PostFormValue("importance"),
		r.PostFormValue("goal"),
	)
	if err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/tasks/"+rec.Meta.ID, http.StatusSeeOther)
}

// ---------- SSE / JSON ----------

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := s.broker.subscribe()
	defer s.broker.unsubscribe(ch)

	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()
	ctx := r.Context()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			fmt.Fprint(w, "event: changed\ndata: {}\n\n")
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) handleAPITasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.V.ListTasks(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tasks)
}

func (s *Server) handleAPINext(w http.ResponseWriter, r *http.Request) {
	rec, err := s.V.Next()
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "queue empty"})
		return
	}
	writeJSON(w, rec.Meta)
}

func (s *Server) handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.V.ListPresence()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []store.Presence{}
	}
	writeJSON(w, agents)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// ---------- 视图构造 ----------

func buildTaskView(r *store.Record, act string) taskView {
	v := taskView{R: r, Actor: act}
	var extra strings.Builder
	for _, sec := range store.Sections(r.Body) {
		switch sec.Title {
		case "描述":
			v.DescriptionHTML = mdToHTML(sec.Content)
		case "验收标准":
			n := 0
			for _, line := range strings.Split(sec.Content, "\n") {
				t := strings.TrimSpace(line)
				checked := false
				var rest string
				if strings.HasPrefix(t, "- [x] ") || strings.HasPrefix(t, "- [X] ") {
					checked, rest = true, t[6:]
				} else if strings.HasPrefix(t, "- [ ] ") {
					rest = t[6:]
				} else {
					continue
				}
				if strings.TrimSpace(rest) == "" {
					continue
				}
				n++
				if checked {
					v.AcceptDone++
				}
				v.Accept = append(v.Accept, acceptItem{N: n, Checked: checked, HTML: mdToHTML(rest)})
			}
		case "日志":
			for _, line := range strings.Split(sec.Content, "\n") {
				t := strings.TrimSpace(line)
				if strings.HasPrefix(t, "- ") {
					v.Logs = append(v.Logs, strings.TrimPrefix(t, "- "))
				}
			}
			// 最新日志排在最前
			for i, j := 0, len(v.Logs)-1; i < j; i, j = i+1, j-1 {
				v.Logs[i], v.Logs[j] = v.Logs[j], v.Logs[i]
			}
		case "":
			// 序言，忽略
		default:
			extra.WriteString("<h3>" + template.HTMLEscapeString(sec.Title) + "</h3>\n")
			extra.WriteString(string(mdToHTML(sec.Content)))
		}
	}
	v.Extra = template.HTML(extra.String()) //nolint:gosec // 内容来自用户本人的本地 vault
	return v
}

var md = goldmark.New()

func mdToHTML(src string) template.HTML {
	var buf strings.Builder
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src)) //nolint:gosec
	}
	return template.HTML(strings.TrimSpace(buf.String())) //nolint:gosec
}

// ---------- 模板与小工具 ----------

var funcMap = template.FuncMap{
	"statusZH": statusZH,
	"impZH":    impZH,
	"impClass": impClass,
	"baseName": filepath.Base,
	"ago":      agoText,
	"actorName": func(s string) string {
		if i := strings.Index(s, "@"); i > 0 {
			return s[:i]
		}
		return s
	},
}

var boardTmpl = template.Must(template.New("board").Funcs(funcMap).Parse(boardHTML))
var taskTmpl = template.Must(template.New("task").Funcs(funcMap).Parse(taskHTML))

func render(w http.ResponseWriter, code int, tmpl *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_ = tmpl.Execute(w, data)
}

func actor(r *http.Request) string {
	if c, err := r.Cookie(actorCookie); err == nil && c.Value != "" {
		return c.Value
	}
	return "jmzx@studio"
}

func redirectErr(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/?error="+url.QueryEscape(msg), http.StatusSeeOther)
}

func backToBoardOrTask(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.PostFormValue("from") == "task" {
		http.Redirect(w, r, "/tasks/"+r.PathValue("id"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func atoiDefault(s string, def int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if s == "" {
		return def
	}
	return n
}

func statusZH(s string) string {
	switch s {
	case store.StatusDraft:
		return "草稿"
	case store.StatusReady:
		return "待领取"
	case store.StatusInProgress:
		return "进行中"
	case store.StatusBlocked:
		return "受阻"
	case store.StatusDone:
		return "已归港"
	case store.StatusCancelled:
		return "已取消"
	}
	return s
}

func impZH(s string) string {
	switch s {
	case store.ImpCritical:
		return "关键"
	case store.ImpHigh:
		return "重要"
	case store.ImpSomeday:
		return "也许"
	}
	return "普通"
}

func impClass(s string) string { return "imp-" + s }

// agoText 渲染心跳的相对时间：刚刚 / x 分钟前 / x 小时前。
func agoText(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
}

func serveStatic(name, ctype string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := staticFS.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		_, _ = w.Write(b)
	}
}
