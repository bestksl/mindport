// 按目录翻会话（简单粗暴版）：不依赖心跳，给一个项目绝对路径，
// 直接扫 ~/.claude/projects/<编码目录>/ 列出全部历史会话，点进船台。
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/bestksl/mindport/internal/store"
	"github.com/bestksl/mindport/internal/transcript"
)

type dirRow struct {
	transcript.SessionMeta
	Online bool
	Status string // busy | waiting | idle | ""（离线）
	Name   string // 官方发现口里的会话简介（若有）
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
	NoDir     bool // 没传 path：展示输入框 + 已知目录
	Error     string
}

func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	d := dirListPage{
		Agents:    s.listPresence(),
		Auth:      s.authOn(),
		KnownDirs: s.knownDirs(),
		NoDir:     true,
	}
	path := r.URL.Query().Get("dir")
	if path == "" {
		render(w, http.StatusOK, dirTmpl, d)
		return
	}
	d.NoDir = false
	d.Path = filepath.Clean(path)
	d.Short = filepath.Base(d.Path)
	if !filepath.IsAbs(d.Path) {
		d.Error = "要给项目目录的绝对路径（如 /Users/jmzx/Documents/GitHub/mindport）"
		render(w, http.StatusBadRequest, dirTmpl, d)
		return
	}
	if st, err := os.Stat(d.Path); err != nil || !st.IsDir() {
		d.Error = "本机找不到这个目录，确认是跑 Claude Code 的那台机器上的绝对路径"
		render(w, http.StatusNotFound, dirTmpl, d)
		return
	}
	home, _ := os.UserHomeDir()
	metas, err := transcript.ListSessions(home, d.Path)
	if err != nil {
		d.Error = "这个目录下还没有 Claude 会话记录（~/.claude/projects 里查无此目录）"
		render(w, http.StatusNotFound, dirTmpl, d)
		return
	}
	var live map[string]cliSession
	if s.poller != nil {
		live = s.poller.snapshot()
	}
	for _, m := range metas {
		row := dirRow{SessionMeta: m}
		if ls, ok := live[m.SID]; ok {
			row.Online = true
			row.Status = ls.Status
			row.Name = ls.Name
		}
		d.Rows = append(d.Rows, row)
	}
	render(w, http.StatusOK, dirTmpl, d)
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
