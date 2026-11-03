// bridge_actions：网页会话的双向动作——即时说话、新建会话、中断、撤回、
// 删除会话，以及把桥事件实时推给浏览器的 /api/live SSE。
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bestksl/mindport/internal/store"
	"github.com/bestksl/mindport/internal/transcript"
)

// maxFleetNameRunes 会话名长度上限（短码-需求，保持列表一行放下）。
const maxFleetNameRunes = 40

// handleSessionSay 优先走常驻桥：活桥即时注入（忙时 CLI 自动排队），
// 没桥就当场为这个会话续一座桥（--resume）。桥彻底不可用时才退回旧留言。
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
	if len([]rune(msg)) > 8000 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "话太长了（8000 字以内）"})
		return
	}
	info, found := s.resolveSession(sid, r.URL.Query().Get("dir"))
	if s.isHiddenFleet(sid) || !found || info.dir == "" {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	if info.anon {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "这个会话还没说过话；请在它自己的终端里开口，或新开一个会话"})
		return
	}
	// 又开口说话 = 结局翻篇：旧的「圆满结束/中途停止」标记自动撤掉。
	_ = s.V.UnmarkSession(sid)

	if _, alive := s.bridge.stateOf(sid); alive {
		// 豆包式「改发新请求」：忙时发送 = 立即中断当前轮，用新话点火新一轮（旧排队丢弃）。
		if r.PostFormValue("stop_first") == "1" {
			if st, _ := s.bridge.stateOf(sid); st == bridgeWorking || st == bridgeStarting || st == bridgeWaiting {
				if err := s.bridge.ReplaceTurn(sid, msg); err == nil {
					writeJSON(w, sayResp{Delivered: "live", Detail: "已停止上一轮，按你新的这句开跑"})
					return
				}
			}
		}
		if queued, err := s.bridge.Say(sid, msg); err == nil {
			n := 0
			if queued {
				if _, _, q, _ := s.bridge.snapshot(sid); len(q) > 0 {
					n = len(q)
				}
			}
			detail := "已送达，会话立刻接着跑"
			if queued {
				detail = "会话正忙，话已排队，这轮跑完马上接着说"
			}
			writeJSON(w, sayResp{Delivered: "live", Pending: n, Detail: detail})
			return
		}
		// stdin 写坏（桥半死）：落到下面重起一座桥。
	}

	_, _, err := s.bridge.Start(info.dir, sid, "", msg)
	if err != nil && !errors.Is(err, errBridgeWarming) {
		// 会话正被终端/别的客户端占着：不报错，排队等席位空出自动续（t3 queue 语义）。
		if errors.Is(err, errSessionHeldElsewhere) {
			s.queueHeld(w, sid, info.dir, msg)
			return
		}
		// 仅当 CLI 真的起不来（找不到可执行文件/登录态彻底坏）才退回留言；
		// 慢唤醒不算失败——话已在桥进程 stdin 里，不能再落留言造成重复/黑洞。
		s.queueMail(w, sid, msg)
		return
	}
	if _, ok := s.V.Fleet(sid); !ok {
		_ = s.V.SaveFleet(store.FleetInfo{SID: sid, Dir: info.dir, Name: info.name})
	}
	if s.poller != nil {
		go s.poller.poll()
	}
	if errors.Is(err, errBridgeWarming) {
		writeJSON(w, sayResp{Delivered: "live", Detail: "正在唤醒电脑端会话（历史较长，读一会儿就接着跑），话已送达"})
		return
	}
	writeJSON(w, sayResp{Delivered: "live", Detail: "已唤醒会话，正在接着跑"})
}

// regenResp 报告「重新生成」取回的原话，供前端做重复气泡去重。
type regenResp struct {
	OK           string `json:"ok"`
	Regenerating string `json:"regenerating"`
	Detail       string `json:"detail,omitempty"`
}

// handleSessionRegenerate 豆包式「重新生成」：取回最近一句真人原话，中断当前轮，
// 让新桥以同一句话重新作答（线性 transcript 无法云端分叉，等价终端 Esc 后 ↑回车）。
func (s *Server) handleSessionRegenerate(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	info, found := s.resolveSession(sid, r.URL.Query().Get("dir"))
	if s.isHiddenFleet(sid) || !found || info.dir == "" {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	if info.anon {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "这个会话还没说过话"})
		return
	}
	p, ok := s.transcriptPath(info, sid)
	if !ok {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "还没有对话记录，无法重新生成"})
		return
	}
	last, ok2 := transcript.LastHumanText(p)
	if !ok2 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "找不到上一句真人发言"})
		return
	}
	_ = s.V.UnmarkSession(sid) // 重新作答 = 旧结局翻篇

	if _, alive := s.bridge.stateOf(sid); alive {
		if err := s.bridge.ReplaceTurn(sid, last); err != nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]string{"error": "没停住当前轮，稍后重试：" + err.Error()})
			return
		}
		writeJSON(w, regenResp{OK: "1", Regenerating: last, Detail: "已让会话重新作答"})
		return
	}
	// 桥不在（离线/已停）：resume 起一座新桥用原话点火。
	_, _, err := s.bridge.Start(info.dir, sid, "", last)
	if err != nil && !errors.Is(err, errBridgeWarming) {
		if errors.Is(err, errSessionHeldElsewhere) {
			s.queueHeld(w, sid, info.dir, last)
			return
		}
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]string{"error": "唤不起会话：" + err.Error()})
		return
	}
	if _, ok := s.V.Fleet(sid); !ok {
		_ = s.V.SaveFleet(store.FleetInfo{SID: sid, Dir: info.dir, Name: info.name})
	}
	if s.poller != nil {
		go s.poller.poll()
	}
	writeJSON(w, regenResp{OK: "1", Regenerating: last, Detail: "正在唤醒会话重新作答"})
}

// handleFleetNew 一键新开会话：目录 + 会话名（短码-需求）+ 首句话，
// 可选挂目标/任务。返回 JSON 供前端就地报错与跳转，避免旧版 launch
// 「失败踢回首页丢表单 + 双击开出两个会话」。
func (s *Server) handleFleetNew(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	dir := filepath.Clean(strings.TrimSpace(r.PostFormValue("dir")))
	if !filepath.IsAbs(dir) {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "要给项目目录的绝对路径，会话才知道在哪儿开工"})
		return
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "本机找不到这个目录：" + dir})
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" || utf8.RuneCountInString(name) > maxFleetNameRunes || strings.ContainsAny(name, "\n\r") {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "会话名必填，最多 " + fmt.Sprint(maxFleetNameRunes) + " 个字、不能换行"})
		return
	}
	msg := strings.TrimSpace(r.PostFormValue("msg"))
	if len([]rune(msg)) > 8000 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "开场白太长（8000 字以内）"})
		return
	}
	routeID := strings.TrimSpace(r.PostFormValue("route"))
	taskID := strings.TrimSpace(r.PostFormValue("task"))
	if routeID != "" {
		if _, err := s.V.GetRoute(routeID); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	if taskID != "" {
		if _, err := s.V.GetTask(taskID); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	sid, _, err := s.bridge.Start(dir, "", name, msg)
	if err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]string{"error": "新开会话失败：" + err.Error()})
		return
	}
	if err := s.V.SaveFleet(store.FleetInfo{
		SID: sid, Name: name, Dir: dir, Task: taskID, Route: routeID,
	}); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "会话名册登记失败：" + err.Error()})
		return
	}
	if taskID != "" {
		if _, err := s.V.BindSession(taskID, sid, dir, dispatcher); err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "会话已开但关联任务失败：" + err.Error()})
			return
		}
		if routeID != "" {
			_ = s.V.SetTaskRoute(taskID, routeID)
		}
	}
	if s.poller != nil {
		go s.poller.poll()
	}
	writeJSON(w, map[string]string{
		"ok":  "1",
		"sid": sid,
		"url": "/sessions/" + sid + "?dir=" + url.QueryEscape(dir),
	})
}

// handleFleetStop 中断会话当前轮（网页版 Esc）；cancel_all=1 连排队的话一起撤。
func (s *Server) handleFleetStop(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	_ = r.ParseForm()
	cancelAll := r.PostFormValue("cancel_all") == "1"
	if _, alive := s.bridge.stateOf(sid); !alive {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "只有网页开的会话能在网页中断；终端里开的会话请回终端按 Esc"})
		return
	}
	if err := s.bridge.Interrupt(sid, cancelAll); err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]string{"error": "中断指令没送进去：" + err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "cancel_all": cancelAll})
}

// handleFleetDelete 删除会话（等效 CC 的 Ctrl-X）：停桥进程 + 从思港抹掉。
// 不删 CC transcript——历史文件归 CC 自己管。
func (s *Server) handleFleetDelete(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	info, _ := s.resolveSession(sid, r.URL.Query().Get("dir"))
	s.bridge.Kill(sid)
	name := ""
	if fi, ok := s.V.Fleet(sid); ok {
		name = fi.Name
	}
	if err := s.V.HideFleet(sid, info.dir, name); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = s.V.DropMail(sid)
	_ = s.V.UnmarkSession(sid)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) isHiddenFleet(sid string) bool {
	return s.V.HiddenSIDs()[sid]
}

// handlePerms 列出待审批（?sid= 只看这个会话；省略=全部页面角标用）。
func (s *Server) handlePerms(w http.ResponseWriter, r *http.Request) {
	if s.perm == nil {
		writeJSON(w, map[string]any{"perms": []PermView{}, "enabled": false})
		return
	}
	sid := r.URL.Query().Get("sid")
	mode := ModeAuto
	if sid != "" {
		mode = s.perm.Mode(sid)
	}
	writeJSON(w, map[string]any{
		"perms":     s.perm.views(sid),
		"enabled":   true,
		"mode":      mode,
		"autopilot": mode == ModeYolo,
	})
}

// handlePermDecide 手机拍板：decision=allow|deny。
func (s *Server) handlePermDecide(w http.ResponseWriter, r *http.Request) {
	if s.perm == nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]string{"error": "手机审批通道未开启"})
		return
	}
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	_ = r.ParseForm()
	id := strings.TrimSpace(r.PostFormValue("id"))
	decision := strings.TrimSpace(r.PostFormValue("decision"))
	if id == "" || (decision != "allow" && decision != "deny") {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "要给审批 id 和 allow/deny"})
		return
	}
	if !s.perm.decide(id, decision) {
		writeJSONStatus(w, http.StatusGone, map[string]string{"error": "这条审批已收束（可能已超时拒绝）"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "decision": decision})
}

// handleMode 切换一个会话的权限模式（auto/plan/yolo），对齐 Claude Code 的 Mode。
// mode 是 CLI 启动参数，常驻桥要换代才生效：中断旧代后以新 --permission-mode
// --resume 起一个空闲桥（无点火句）；若桥本来就没起（冷态/已退役），只存模式，
// 下次说话自然按新模式冷启。
func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	if s.perm == nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]string{"error": "手机审批通道未开启"})
		return
	}
	sid := r.PathValue("sid")
	if !sidOK(sid) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	_ = r.ParseForm()
	mode := normMode(strings.TrimSpace(r.PostFormValue("mode")))
	s.perm.setMode(sid, mode)
	restarted := false
	if s.bridge != nil {
		if _, alive := s.bridge.stateOf(sid); alive {
			// 纯中断→以新模式 --resume 空闲等待。换代失败（如席位被终端占着）模式也已存下，
			// 下轮冷启生效；不算硬错误，把情况作为 note 还给前端。
			if err := s.bridge.ReplaceTurn(sid, ""); err != nil {
				writeJSON(w, map[string]any{"ok": true, "mode": mode, "restarted": false, "note": err.Error()})
				return
			}
			restarted = true
		}
	}
	writeJSON(w, map[string]any{"ok": true, "mode": mode, "restarted": restarted})
}

// handleLive 是桥事件的统一 SSE 出口：会话页收 state/partial（逐字直播），
// 会话列表/角标收 fleet（翻灯即刷）。
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ch := s.bridge.subscribe()
	defer s.bridge.unsubscribe(ch)
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
