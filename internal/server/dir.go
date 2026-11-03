// 按目录翻会话（简单粗暴版）：不依赖在线状态，给一个项目绝对路径，
// 直接扫 ~/.claude/projects/<编码目录>/ 列出全部历史会话，点进会话页。
// 不给路径时聚合「全部项目的最近会话」（路径由 transcript cwd 自证），零输入即用。
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bestksl/mindport/internal/store"
	"github.com/bestksl/mindport/internal/transcript"
)

const (
	recentScanFiles = 80 // 聚合最近会话时最多扫多少个 transcript 文件（按 mtime）
	recentTotal     = 40 // 聚合列表最多返回多少条
)

type dirRow struct {
	transcript.SessionMeta
	Proj      string // 项目目录 basename（聚合视图跨目录时显示）
	Class     string // 行状态 class：is-working/is-waiting/is-idle/is-off
	StateText string // 工作中 / 待你输入 / 已停止
	MarkText  string // ✅ 圆满结束 / ⏹ 中途停止（有标记时覆盖状态文案）
	ShowProj  bool   // 聚合视图跨目录：行内显示项目名
	HideCount bool   // 聚合只扫文件头：隐藏消息计数
	Online    bool
	Status    string // busy | waiting | idle | ""（离线）
	Mark      string // good | stop | ""
	Name      string // 官方发现口里的会话简介（若有）
	Anon      bool   // 进程在跑但还没说话：无 transcript、sid 是 proc-<pid>
	Perms     int    // 等手机拍板的工具审批数（v0.9）
}

type knownDir struct {
	Dir   string
	Short string
	Name  string
}

type dirListPage struct {
	Agents    []store.Presence
	Auth      bool
	Path      string
	Short     string
	Rows      []dirRow
	Groups    map[string][]dirRow // working|waiting|idle|done|offline 单栏分组
	KnownDirs []knownDir
	Routes    []store.Record // 新建会话时可关联的在役目标线
	RecentDir []string       // 本机最近浏览（前端 localStorage 注入，服务端只给在线状态目录）
	NoDir     bool           // 没传 dir：展示输入框 + 已知目录 + 最近会话聚合
	Error     string
}

// groupFleetRows 把会话行按「进行中 / 待输入 / 待命 / 完成 / 历史」分五组，
// 单栏表达全部在用会话；完成（圆满结束）默认折叠。
func groupFleetRows(rows []dirRow) map[string][]dirRow {
	g := map[string][]dirRow{}
	for _, r := range rows {
		key := "offline"
		switch {
		case r.Mark == store.MarkGood:
			key = "done"
		case r.Anon:
			key = "waiting"
		case r.Online && (r.Status == "busy" || r.Status == "restarting"):
			key = "working"
		case r.Online && r.Status == "waiting":
			key = "waiting"
		case r.Online:
			key = "idle"
		}
		g[key] = append(g[key], r)
	}
	return g
}

// sessionRows 是 HTML 与 JSON 两个出口共用的行构造：dir 为空聚合最近会话，
// 否则列指定目录；并合并 poller 在线快照标状态点。
func (s *Server) sessionRows(home, dir string) ([]dirRow, error) {
	var metas []transcript.SessionMeta
	var err error
	if dir == "" {
		metas, err = transcript.RecentSessions(home, recentScanFiles, recentTotal)
	} else {
		metas, err = transcript.ListSessions(home, dir)
	}
	if err != nil && err != transcript.ErrNotFound {
		return nil, err
	}
	// ErrNotFound 也继续：目录里可能正有一个还没说过话的匿名 TUI（extras 补上），
	// 不能因为没有 transcript 文件就把实时会话整个 404 掉。
	var live map[string]cliSession
	if s.poller != nil {
		live = s.poller.snapshot()
	}
	marks := s.V.SessionMarks()
	rows := make([]dirRow, 0, len(metas))
	for _, m := range metas {
		row := dirRow{SessionMeta: m, Proj: filepath.Base(m.Dir), ShowProj: dir == "", HideCount: dir == ""}
		if ls, ok := live[m.SID]; ok {
			row.Online = true
			row.Status = ls.Status
			row.Name = ls.Name
		}
		fs := fleetStateOf(row.Online, row.Status)
		row.Class = "is-" + fs.Key
		row.StateText = fs.Text
		if mk, ok := marks[m.SID]; ok {
			row.Mark = mk.Outcome
			row.MarkText = markText(mk.Outcome)
		}
		rows = append(rows, row)
	}

	// 在线但扫盘没覆盖到的实时会话也要算会话：桌面 app 会话（--agent）、
	// 刚开还没说话的 TUI（匿名 proc-<pid>）、掉出最近窗口仍在线的老会话。
	have := map[string]bool{}
	for _, r := range rows {
		have[r.SID] = true
	}
	var extras []dirRow
	for sid, ls := range live {
		if have[sid] {
			continue
		}
		// 目录视图只收本目录的会话：非匿名会话以 transcript 记录里自证的
		// cwd 为准（桌面 app 的进程 cwd 不一定是项目目录），匿名行只能信进程 cwd。
		if dir != "" {
			eff := ls.CWD
			if !ls.Anon {
				if m := transcript.Meta(home, ls.CWD, sid); m.Dir != "" {
					eff = m.Dir
				}
			}
			if eff != dir {
				continue
			}
		}
		row := dirRow{
			Online: true, Status: ls.Status, Name: ls.Name,
			Proj: filepath.Base(ls.CWD), ShowProj: dir == "", HideCount: true,
		}
		if ls.Anon {
			row.Anon = true
			row.SID = sid
			row.Dir = ls.CWD
			row.Title = "新开的会话（还没说话）"
			row.LastTS = ls.StartedAtTime() // 展示用：进程启动时刻
		} else {
			row.SessionMeta = transcript.Meta(home, ls.CWD, sid)
			if ls.CWD != "" && row.Dir == "" {
				row.Dir = ls.CWD
			}
			if ls.Name != "" {
				row.Title = ls.Name // 发现口简介优先于首句标题
			}
		}
		fs := fleetStateOf(true, ls.Status)
		row.Class = "is-" + fs.Key
		row.StateText = fs.Text
		if mk, ok := marks[sid]; ok {
			row.Mark = mk.Outcome
			row.MarkText = markText(mk.Outcome)
		}
		extras = append(extras, row)
	}
	sort.SliceStable(extras, func(i, j int) bool {
		if extras[i].Anon != extras[j].Anon {
			return !extras[i].Anon
		}
		return extras[i].Title < extras[j].Title
	})
	return s.enrichRows(append(extras, rows...), home), nil
}

// expandTilde 把 ~/foo 展开成 home/foo；其余原样返回。
func expandTilde(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	routes, _ := s.V.ListRoutes()
	activeRoutes := routes[:0]
	for _, rt := range routes {
		if rt.Meta.Status != store.RouteArchived {
			activeRoutes = append(activeRoutes, rt)
		}
	}
	d := dirListPage{
		Agents:    s.listPresence(),
		Auth:      s.authOn(),
		KnownDirs: s.knownDirs(),
		Routes:    activeRoutes,
		NoDir:     true,
	}
	home, _ := os.UserHomeDir()
	path := r.URL.Query().Get("dir")

	if path == "" {
		// 空态也给内容：全部项目最近会话，一进来就能点
		if rows, err := s.sessionRows(home, ""); err == nil {
			d.Rows = rows
			d.Groups = groupFleetRows(rows)
		}
		render(w, http.StatusOK, dirTmpl, d)
		return
	}

	d.NoDir = false
	d.Path = filepath.Clean(expandTilde(path, home))
	d.Short = filepath.Base(d.Path)
	if !filepath.IsAbs(d.Path) {
		d.Error = "要给项目目录的绝对路径（如 /Users/jmzx/Documents/GitHub/mindport，~/ 也行）"
		render(w, http.StatusBadRequest, dirTmpl, d)
		return
	}
	if st, err := os.Stat(d.Path); err != nil || !st.IsDir() {
		d.Error = "本机找不到这个目录，确认是跑 Claude Code 的那台机器上的绝对路径"
		render(w, http.StatusNotFound, dirTmpl, d)
		return
	}
	rows, err := s.sessionRows(home, d.Path)
	if err != nil || len(rows) == 0 {
		d.Error = "这个目录下还没有 Claude 会话记录（~/.claude/projects 里查无此目录）"
		render(w, http.StatusNotFound, dirTmpl, d)
		return
	}
	d.Rows = rows
	d.Groups = groupFleetRows(rows)
	render(w, http.StatusOK, dirTmpl, d)
}

// 列表轻量轮询：只回状态/计数/结局标记，前端 8s 静默刷状态点，不重载整页。
type sessionRowJSON struct {
	SID    string `json:"sid"`
	Dir    string `json:"dir"`
	Title  string `json:"title"`
	Name   string `json:"name"` // 会话名（-n「短码-需求」，名册优先）
	Proj   string `json:"proj"`
	Events int    `json:"events"`
	Online bool   `json:"online"`
	Status string `json:"status"`
	Mark   string `json:"mark"`
	Anon   bool   `json:"anon"`
	Perms  int    `json:"perms"` // 待手机审批数
	Stamp  int64  `json:"stamp"` // LastTS unnix，组内排序用
}

func (s *Server) handleSessionListJSON(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	dir := r.URL.Query().Get("dir")
	if dir != "" {
		dir = filepath.Clean(expandTilde(dir, home))
		if !filepath.IsAbs(dir) {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "路径要绝对路径"})
			return
		}
	}
	rows, err := s.sessionRows(home, dir)
	if err != nil || (dir != "" && len(rows) == 0) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "查无会话"})
		return
	}
	out := make([]sessionRowJSON, 0, len(rows))
	for _, r2 := range rows {
		stamp := int64(0)
		if !r2.LastTS.IsZero() {
			stamp = r2.LastTS.Unix()
		}
		out = append(out, sessionRowJSON{
			SID: r2.SID, Dir: r2.Dir, Title: r2.Title, Name: r2.Name, Proj: r2.Proj,
			Events: r2.Events, Online: r2.Online, Status: r2.Status, Mark: r2.Mark, Anon: r2.Anon, Perms: r2.Perms, Stamp: stamp,
		})
	}
	writeJSON(w, map[string]any{"rows": out})
}

// knownDirs 从在线状态里收集去重后的项目目录，作为空态页的快捷入口。
func (s *Server) knownDirs() []knownDir {
	seen := map[string]bool{}
	var out []knownDir
	for _, p := range s.listPresence() {
		if p.Dir == "" || seen[p.Dir] {
			continue
		}
		seen[p.Dir] = true
		out = append(out, knownDir{Dir: p.Dir, Short: filepath.Base(p.Dir), Name: p.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out
}
