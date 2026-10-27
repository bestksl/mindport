// 按目录翻会话（简单粗暴版）：不依赖心跳，给一个项目绝对路径，
// 直接扫 ~/.claude/projects/<编码目录>/ 列出全部历史会话，点进船台。
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
	KnownDirs []knownDir
	RecentDir []string // 本机最近浏览（前端 localStorage 注入，服务端只给心跳目录）
	NoDir     bool     // 没传 dir：展示输入框 + 已知目录 + 最近会话聚合
	Error     string
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
	if err != nil {
		return nil, err
	}
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
	return rows, nil
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
	d := dirListPage{
		Agents:    s.listPresence(),
		Auth:      s.authOn(),
		KnownDirs: s.knownDirs(),
		NoDir:     true,
	}
	home, _ := os.UserHomeDir()
	path := r.URL.Query().Get("dir")

	if path == "" {
		// 空态也给内容：全部项目最近会话，一进来就能点
		if rows, err := s.sessionRows(home, ""); err == nil {
			d.Rows = rows
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
	if err != nil {
		d.Error = "这个目录下还没有 Claude 会话记录（~/.claude/projects 里查无此目录）"
		render(w, http.StatusNotFound, dirTmpl, d)
		return
	}
	d.Rows = rows
	render(w, http.StatusOK, dirTmpl, d)
}

// 列表轻量轮询：只回状态/计数/结局标记，前端 8s 静默刷状态点，不重载整页。
type sessionRowJSON struct {
	SID    string `json:"sid"`
	Dir    string `json:"dir"`
	Title  string `json:"title"`
	Events int    `json:"events"`
	Online bool   `json:"online"`
	Status string `json:"status"`
	Mark   string `json:"mark"`
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
	if err != nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "查无会话"})
		return
	}
	out := make([]sessionRowJSON, 0, len(rows))
	for _, r2 := range rows {
		out = append(out, sessionRowJSON{
			SID: r2.SID, Dir: r2.Dir, Title: r2.Title,
			Events: r2.Events, Online: r2.Online, Status: r2.Status, Mark: r2.Mark,
		})
	}
	writeJSON(w, map[string]any{"rows": out})
}

// knownDirs 从心跳里收集去重后的项目目录，作为空态页的快捷入口。
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
