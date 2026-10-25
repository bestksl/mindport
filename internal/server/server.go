// Package server 用标准库 net/http 把 vault 同时暴露为极简 Web 页面
// （手机浏览器/PWA）与 JSON API。静态资源经 go:embed 打进同一二进制。
package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/bestksl/mindport/internal/store"
)

//go:embed static/*
var staticFS embed.FS

// Server 持有 vault 引用。
type Server struct {
	V *store.Vault
}

type pageData struct {
	Now      time.Time
	Thoughts []store.Record
	Tasks    []store.Record
	Error    string
}

// Routes 构造路由器（Go 1.22+ 方法与路径变量）。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /thoughts", s.handleCreateThought)
	mux.HandleFunc("POST /tasks", s.handleCreateTask)
	mux.HandleFunc("POST /tasks/{id}/claim", s.handleClaim)
	mux.HandleFunc("POST /tasks/{id}/done", s.handleDone)
	mux.HandleFunc("GET /api/tasks", s.handleAPITasks)

	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /manifest.webmanifest", serveStatic("static/manifest.webmanifest", "application/manifest+json"))
	mux.HandleFunc("GET /sw.js", serveStatic("static/sw.js", "application/javascript"))
	mux.HandleFunc("GET /icon.svg", serveStatic("static/icon.svg", "image/svg+xml"))

	return logRequests(mux)
}

func (s *Server) loadData(errMsg string) pageData {
	d := pageData{Now: time.Now(), Error: errMsg}
	d.Thoughts, _ = s.V.ListThoughts()
	d.Tasks, _ = s.V.ListTasks("")
	return d
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	render(w, http.StatusOK, s.loadData(r.URL.Query().Get("error")))
}

func (s *Server) handleCreateThought(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	if _, err := s.V.AddThought(r.PostFormValue("title"), "", "mobile"); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	if _, err := s.V.NewTask(r.PostFormValue("title")); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	if _, err := s.V.Claim(r.PathValue("id"), "human@vs3"); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleDone(w http.ResponseWriter, r *http.Request) {
	if _, err := s.V.Done(r.PathValue("id"), "human@vs3"); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleAPITasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.V.ListTasks(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	metas := make([]store.Meta, 0, len(tasks))
	for _, t := range tasks {
		metas = append(metas, t.Meta)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(metas)
}

func redirectErr(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/?error="+url.QueryEscape(msg), http.StatusSeeOther)
}

var indexTmpl *template.Template

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#0f2233">
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/icon.svg">
<link rel="apple-touch-icon" href="/icon.svg">
<link rel="stylesheet" href="/static/style.css">
<title>思港 MindPort</title>
</head>
<body>
<header>
  <h1>思港 <span>MindPort</span></h1>
  <p class="tagline">念头会消失，但港口一直在</p>
</header>

{{if .Error}}<div class="err">⚠️ {{.Error}}</div>{{end}}

<section class="capture">
  <form method="post" action="/thoughts">
    <input name="title" placeholder="念头闪现，先扔进港…" autocomplete="off" autofocus required>
    <button type="submit" class="primary">入港</button>
  </form>
  <form method="post" action="/tasks" class="taskform">
    <input name="title" placeholder="或直接建一个正式任务…" autocomplete="off">
    <button type="submit">建任务</button>
  </form>
</section>

<section>
  <h2>⚓ 泊位任务 <small>{{len .Tasks}}</small></h2>
  {{if not .Tasks}}<p class="empty">港里还没有任务</p>{{end}}
  {{range .Tasks}}
  <article class="card status-{{.Meta.Status}}">
    <div class="card-head">
      <span class="id">{{.Meta.ID}}</span>
      <span class="status">{{statusZH .Meta.Status}}</span>
      {{if .Meta.Assignee}}<span class="assignee">⛟ {{.Meta.Assignee}}</span>{{end}}
    </div>
    <div class="title">{{.Meta.Title}}</div>
    <div class="actions">
      {{if eq .Meta.Status "ready"}}
      <form method="post" action="/tasks/{{.Meta.ID}}/claim"><button>领取</button></form>
      {{else if eq .Meta.Status "in_progress"}}
      <form method="post" action="/tasks/{{.Meta.ID}}/done"><button class="primary">归港</button></form>
      {{end}}
    </div>
  </article>
  {{end}}
</section>

<section>
  <h2>🌊 Inbox 闪念 <small>{{len .Thoughts}}</small></h2>
  {{if not .Thoughts}}<p class="empty">还没有接住的闪念</p>{{end}}
  {{range .Thoughts}}
  <article class="card thought">
    <div class="title">{{.Meta.Title}}</div>
    <div class="card-head"><span class="assignee">{{.Meta.Created.Format "01-02 15:04"}}{{if .Meta.Source}} · {{.Meta.Source}}{{end}}</span></div>
  </article>
  {{end}}
</section>

<footer>{{.Now.Format "2006-01-02 15:04:05"}} · 每 10 秒自动刷新</footer>
<script>
if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js');
setTimeout(()=>location.reload(),10000);
</script>
</body>
</html>`

func render(w http.ResponseWriter, code int, d pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_ = indexTmpl.Execute(w, d)
}

func init() {
	indexTmpl = template.Must(template.New("index").Funcs(template.FuncMap{
		"statusZH": func(s string) string {
			switch s {
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
			default:
				return s
			}
		},
	}).Parse(indexHTML))
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

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
