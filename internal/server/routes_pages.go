// 航线页（v0.8）：/routes 列全部航线；/routes/R0001 是一条航线的工作台——
// 描述、临时需求（done 默认收起）、从这条航线开出去的船队。
package server

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/bestksl/mindport/internal/store"
)

type routeListItem struct {
	R          store.Record
	OpenTasks  int
	DoneTasks  int
	FleetCount int
	LiveCount  int // 在航船队数
}

type routeListPage struct {
	Agents []store.Presence
	Auth   bool
	Routes []routeListItem
	Error  string
}

type fleetCard struct {
	FI        store.FleetInfo
	ShortSID  string
	StateKey  string
	StateText string
	Mark      string
}

type routeDetailPage struct {
	Agents    []store.Presence
	Auth      bool
	R         store.Record
	DescHTML  template.HTML
	OpenTasks []store.Record
	DoneTasks []store.Record
	Fleets    []fleetCard
	Dirs      []knownDir
	Error     string
	FromList  bool
}

func (s *Server) handleRouteList(w http.ResponseWriter, r *http.Request) {
	routes, err := s.V.ListRoutes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fleets := s.V.Fleets()
	allTasks, _ := s.V.ListTasks("")
	tasksByRoute := map[string][]store.Record{}
	for _, t := range allTasks {
		if t.Meta.Route != "" {
			tasksByRoute[t.Meta.Route] = append(tasksByRoute[t.Meta.Route], t)
		}
	}
	items := make([]routeListItem, 0, len(routes))
	for _, rt := range routes {
		it := routeListItem{R: rt}
		for _, t := range tasksByRoute[rt.Meta.ID] {
			if t.Meta.Status == store.StatusDone {
				it.DoneTasks++
			} else {
				it.OpenTasks++
			}
		}
		for _, f := range fleets {
			if f.Route == rt.Meta.ID {
				it.FleetCount++
				if st, online := s.bridge.bridgeStatus(f.SID); online && st == "busy" {
					it.LiveCount++
				}
			}
		}
		items = append(items, it)
	}
	render(w, http.StatusOK, routeListTmpl, routeListPage{
		Agents: s.listPresence(),
		Auth:   s.authOn(),
		Routes: items,
		Error:  r.URL.Query().Get("error"),
	})
}

// handleRoutesSummary 是航线 tab 角标的轻量数据源：在役航线数 + 未完成需求数。
func (s *Server) handleRoutesSummary(w http.ResponseWriter, r *http.Request) {
	routes, _ := s.V.ListRoutes()
	tasks, _ := s.V.ListTasks("")
	active, openNeeds := 0, 0
	for _, rt := range routes {
		if rt.Meta.Status != store.RouteArchived {
			active++
		}
	}
	for _, t := range tasks {
		if t.Meta.Route != "" && t.Meta.Status != store.StatusDone {
			openNeeds++
		}
	}
	writeJSON(w, map[string]int{"active": active, "open_needs": openNeeds})
}

func (s *Server) handleRouteCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	title := strings.TrimSpace(r.PostFormValue("title"))
	code := strings.TrimSpace(r.PostFormValue("code"))
	desc := strings.TrimSpace(r.PostFormValue("desc"))
	rec, err := s.V.CreateRoute(title, code, desc)
	if err != nil {
		http.Redirect(w, r, "/routes?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/routes/"+rec.Meta.ID, http.StatusSeeOther)
}

func (s *Server) routeCard(routeID string) []fleetCard {
	var cards []fleetCard
	for _, f := range s.V.Fleets() {
		if f.Route != routeID {
			continue
		}
		card := fleetCard{FI: f, ShortSID: shortSid(f.SID)}
		if st, online := s.bridge.bridgeStatus(f.SID); online {
			fs := fleetStateOf(true, st)
			card.StateKey, card.StateText = fs.Key, fs.Text
		} else if s.poller != nil {
			if ls, ok := s.poller.snapshot()[f.SID]; ok {
				fs := fleetStateOf(true, ls.Status)
				card.StateKey, card.StateText = fs.Key, fs.Text
			}
		}
		if card.StateText == "" {
			if mk, ok := s.V.SessionMark(f.SID); ok {
				card.Mark = mk.Outcome
				card.StateText = markText(mk.Outcome)
				card.StateKey = "mark-" + mk.Outcome
			} else {
				card.StateKey, card.StateText = stateOffline, fleetStateOf(false, "").Text
			}
		}
		cards = append(cards, card)
	}
	return cards
}

func (s *Server) handleRouteDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("rid")
	rec, err := s.V.GetRoute(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tasks, _ := s.V.ListTasksByRoute(id, "")
	d := routeDetailPage{
		Agents:   s.listPresence(),
		Auth:     s.authOn(),
		R:        *rec,
		DescHTML: mdToHTML(routeDesc(rec.Body)),
		Fleets:   s.routeCard(id),
		Dirs:     s.knownDirs(),
	}
	for _, t := range tasks {
		if t.Meta.Status == store.StatusDone {
			d.DoneTasks = append(d.DoneTasks, t)
		} else {
			d.OpenTasks = append(d.OpenTasks, t)
		}
	}
	render(w, http.StatusOK, routeTmpl, d)
}

func (s *Server) handleRouteEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("rid")
	_ = r.ParseForm()
	rec, err := s.V.UpdateRoute(
		id,
		r.PostFormValue("title"),
		r.PostFormValue("code"),
		r.PostFormValue("desc"),
	)
	if err != nil {
		http.Redirect(w, r, "/routes/"+id+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/routes/"+rec.Meta.ID, http.StatusSeeOther)
}

func (s *Server) handleRouteArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("rid")
	_ = r.ParseForm()
	target := store.RouteArchived
	if r.PostFormValue("revive") == "1" {
		target = store.RouteActive
	}
	if err := s.V.SetRouteStatus(id, target); err != nil {
		http.Redirect(w, r, "/routes/"+id+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/routes/"+id, http.StatusSeeOther)
}

// handleRouteNeed 在航线下快速记一个临时需求（草稿任务，带上 route）。
func (s *Server) handleRouteNeed(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("rid")
	_ = r.ParseForm()
	title := strings.TrimSpace(r.PostFormValue("title"))
	if title == "" {
		http.Redirect(w, r, "/routes/"+id+"?error="+url.QueryEscape("需求要写一句要干什么"), http.StatusSeeOther)
		return
	}
	rec, err := s.V.NewTask(title, store.ImpNormal, "", true)
	if err != nil {
		http.Redirect(w, r, "/routes/"+id+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	_ = s.V.SetTaskRoute(rec.Meta.ID, id)
	http.Redirect(w, r, "/tasks/"+rec.Meta.ID, http.StatusSeeOther)
}

// handleTaskSetRoute 任务详情页挂/摘航线。
func (s *Server) handleTaskSetRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.ParseForm()
	route := strings.TrimSpace(r.PostFormValue("route"))
	if err := s.V.SetTaskRoute(id, route); err != nil {
		redirectErr(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/tasks/"+id, http.StatusSeeOther)
}

// routeDesc 从任务同款正文里抽出「## 描述」小节。
func routeDesc(body string) string {
	desc := strings.TrimSpace(body)
	if i := strings.Index(desc, "## 描述"); i >= 0 {
		tail := desc[i+len("## 描述"):]
		if j := strings.Index(tail, "\n## "); j >= 0 {
			tail = tail[:j]
		}
		desc = strings.TrimSpace(tail)
	}
	return desc
}
