// 手机端工具权限审批（v0.9，对标 Happy 的远程 approve）。
//
// 链路：网页桥养的 `claude -p` 以
//
//	--permission-mode manual
//	--mcp-config <每船队一份配置>
//	--permission-prompt-tool mcp__mindport__check_permission
//
// 起。CLI 要执行任何「需要授权」的工具时，会回调我们随船起的一个 stdio MCP
// server（`port _perm-mcp`，即本二进制再入港）。MCP 子进程把请求经 unix
// socket 报给 serve 主进程，主进程经 SSE 把审批卡片推到手机，阻塞等人工
// 「批准/拒绝」：
//
//	allow → 回 {"behavior":"allow"}，CLI 立刻执行；
//	deny  → 回 {"behavior":"deny","message":...}，CLI 干净跳过该工具。
//
// 安全默认：只读工具（Read/Grep/Glob/BashOutput 等）不必打扰手机直接放行；
// 其余一律等人工，5 分钟无人应则默认拒绝；通道任何异常也默认拒绝（绝不静默放行）。
// 想完全不被打扰可在会话页把模式切到「无人值守」(yolo)：回调对该 sid 全部 allow。
//
// 协议来自 CLI 2.1.310 实测（2026-10）：--permission-prompt-tool 取的是一个
// MCP 工具全名（mcp__<server>__<tool>），回调入参
// {"tool_name","input","tool_use_id"}，应答是工具返回 text 里的 JSON
// {"behavior":"allow","updatedInput"?} 或 {"behavior":"deny","message"}。
package server

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// permAskTimeout：人没在手机上应答时，等多久后默认拒绝。
// CLI 会一直阻塞到回调返回，给足看手机的时间，但不能永不收束。
const permAskTimeout = 5 * time.Minute

// permReadOnly 是无需打扰手机、直接放行的只读工具白名单。
// 只列「绝不改文件、不发网络写、不执行任意命令」的；拿不准的一律进人工。
var permReadOnly = map[string]bool{
	"Read":       true,
	"Grep":       true,
	"Glob":       true,
	"BashOutput": true, // 只读 Bash 已产出的输出
	"TodoRead":   true,
}

// permDecision 是回给 CLI 的最终判定（JSON 放进 MCP 工具返回的 text）。
type permDecision struct {
	Behavior string `json:"behavior"`          // allow | deny
	Message  string `json:"message,omitempty"` // deny 时给模型/人的理由
}

// permAskReq 是 MCP 子进程 → serve 主进程的请示帧（每行一帧 JSON）。
type permAskReq struct {
	Type      string          `json:"type"` // 固定 "ask"
	Token     string          `json:"token"`
	SID       string          `json:"sid"`
	ToolUseID string          `json:"tool_use_id"`
	ToolName  string          `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
}

// permAsk 是一条挂起、等手机拍板的审批。
type permAsk struct {
	ID      string    `json:"id"`
	SID     string    `json:"sid"`
	Tool    string    `json:"tool"`
	Summary string    `json:"summary"`
	Detail  string    `json:"detail"`
	Born    time.Time `json:"born"`

	conn     net.Conn
	done     chan struct{}
	decision permDecision
	resolved bool // 防重复拍板（HTTP 双击/超时与人工撞车）
}

// PermView 是推给前端的一条待审批。
type PermView struct {
	ID      string `json:"id"`
	SID     string `json:"sid"`
	Tool    string `json:"tool"`
	Emoji   string `json:"emoji"`
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
	AgeSec  int    `json:"age_sec"`
	Name    string `json:"name,omitempty"`
}

type permManager struct {
	srv      *Server
	sockPath string
	token    string
	exe      string
	cfgDir   string
	ln       net.Listener

	mu      sync.Mutex
	pending map[string]*permAsk
	mode    map[string]string // sid → 权限模式：auto(默认)/plan/yolo
}

// 权限模式，对齐 Claude Code：
//   - ModeAuto：CLI 原生 auto，低风险操作自动干、真正危险的才弹手机（日常默认）；
//   - ModePlan：CLI 原生 plan，只读探索+出计划，不动手改东西；
//   - ModeYolo：CLI manual 但我们的回调对本会话全部 allow（旧「放手/无人值守」，不逐条请批）。
const (
	ModeAuto = "auto"
	ModePlan = "plan"
	ModeYolo = "yolo"
)

// normMode 归一化模式值，非法/空一律落回默认 auto。
func normMode(m string) string {
	switch m {
	case ModePlan, ModeYolo:
		return m
	default:
		return ModeAuto
	}
}

// EnablePerms 由 serve 命令显式开启（测试/精简装配默认关，桥仍走 acceptEdits）。
// 在 vault 的 .port/state 下监听 unix socket，生成一次性令牌，并准备每船队的
// MCP 配置目录。返回错误仅用于打印警告——权限系统起不来不应拖垮整个港。
func (s *Server) EnablePerms() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	state := filepath.Join(s.V.Root, ".port", "state")
	cfgDir := filepath.Join(state, "perm")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return err
	}
	// socket 放系统临时目录并用 vault 路径哈希做短名：unix 域路径有约 104 字节
	// 上限，深 home/测试临时目录下放 vault 里会 bind EINVAL。安全靠 0600+一次性令牌。
	abs, _ := filepath.Abs(s.V.Root)
	h := fnv.New64a()
	_, _ = h.Write([]byte(abs))
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("mp-%x.sock", h.Sum(nil)))
	_ = os.Remove(sock) // 上次没清干净（崩溃/SIGKILL）：先腾位
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	tok, err := randToken(24)
	if err != nil {
		_ = ln.Close()
		return err
	}
	m := &permManager{
		srv: s, sockPath: sock, token: tok, exe: exe, cfgDir: cfgDir,
		ln: ln, pending: map[string]*permAsk{}, mode: map[string]string{},
	}
	s.perm = m
	go m.serveConns()
	log.Printf("perm: 手机审批通道已就绪 socket=%s mcp=%s _perm-mcp", sock, exe)
	return nil
}

func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// permReady 报告审批通道是否在役；spawn 据此决定加不加 MCP 参数。
func (s *Server) permReady() bool { return s.perm != nil && s.perm.ln != nil }

// sessionMode 取一支船队当前权限模式；审批通道未装配（测试/精简）时返回空，
// 由桥退化为 acceptEdits，不把模式判断散落到调用处。
func (s *Server) sessionMode(sid string) string {
	if s.perm == nil {
		return ""
	}
	return s.perm.Mode(sid)
}

func (m *permManager) serveConns() {
	for {
		c, err := m.ln.Accept()
		if err != nil {
			return // 监听器关了（停港）
		}
		go m.handleConn(c)
	}
}

// handleConn：一条连接 = 一次工具请示（MCP 子进程每次 tools/call 新建连接）。
func (m *permManager) handleConn(c net.Conn) {
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	sc := bufio.NewScanner(c)
	if !sc.Scan() {
		return // 没请示就断了
	}
	var req permAskReq
	if json.Unmarshal(sc.Bytes(), &req) != nil || req.Type != "ask" ||
		subtleNotEqual(req.Token, m.token) {
		return // 不是合法请示：直接挂，子进程按连接失败安全拒绝
	}
	_ = c.SetReadDeadline(time.Time{})

	summary, detail := permSummarize(req.ToolName, req.Input)

	// 自动放行：只读工具，或这支船队处于 yolo（无人值守/旧放手）模式。
	m.mu.Lock()
	yolo := normMode(m.mode[req.SID]) == ModeYolo
	m.mu.Unlock()
	if yolo || permReadOnly[req.ToolName] {
		m.writeDecision(c, permDecision{Behavior: "allow"})
		return
	}

	a := &permAsk{
		ID: req.ToolUseID, SID: req.SID, Tool: req.ToolName,
		Summary: summary, Detail: detail, Born: time.Now(),
		conn: c, done: make(chan struct{}),
	}
	m.mu.Lock()
	m.pending[a.ID] = a
	m.mu.Unlock()
	m.publish(a)

	timer := time.NewTimer(permAskTimeout)
	defer timer.Stop()
	select {
	case <-a.done: // 手机拍板，decision 已写在 a 上
	case <-timer.C:
		m.mu.Lock()
		if !a.resolved { // 别在人工刚拍板的同一刻用超时覆盖掉
			a.decision = permDecision{Behavior: "deny",
				Message: "思港：手机端 5 分钟内未批准，已默认拒绝（可在手机上重发该请求）"}
			a.resolved = true
		}
		m.mu.Unlock()
	}
	m.mu.Lock()
	delete(m.pending, a.ID)
	m.mu.Unlock()
	m.writeDecision(c, a.decision)
	m.publish(nil) // 撤卡片 / 刷角标
}

func (m *permManager) writeDecision(c net.Conn, d permDecision) {
	if d.Behavior == "deny" && strings.TrimSpace(d.Message) == "" {
		d.Message = "思港：手机端拒绝了这次操作"
	}
	b, _ := json.Marshal(d)
	_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, _ = c.Write(append(b, '\n'))
}

// publish 把待审批变化推给所有开着的页面（/api/live）。
func (m *permManager) publish(a *permAsk) {
	if m.srv.bridge == nil {
		return
	}
	m.srv.bridge.publish(bridgeEvent{Type: "perm"})
}

// decide 供 HTTP 调用：手机拍板。返回是否找到该条。
func (m *permManager) decide(id, behavior string) bool {
	if behavior != "allow" {
		behavior = "deny"
	}
	m.mu.Lock()
	a := m.pending[id]
	if a != nil && !a.resolved {
		a.resolved = true
		if behavior == "allow" {
			a.decision = permDecision{Behavior: "allow"}
		} else {
			a.decision = permDecision{Behavior: "deny", Message: "思港：手机端拒绝了这次操作"}
		}
	}
	m.mu.Unlock()
	if a == nil {
		return false
	}
	close(a.done)
	return true
}

// views 返回待审批（可按 sid 过滤；sid 空=全部）。
func (m *permManager) views(sid string) []PermView {
	m.mu.Lock()
	now := time.Now()
	out := make([]PermView, 0, len(m.pending))
	for _, a := range m.pending {
		if sid != "" && a.SID != sid {
			continue
		}
		name := ""
		if fi, ok := m.srv.V.Fleet(a.SID); ok {
			name = fi.Name
		}
		out = append(out, PermView{
			ID: a.ID, SID: a.SID, Tool: a.Tool, Emoji: toolEmoji(a.Tool),
			Summary: a.Summary, Detail: a.Detail,
			AgeSec: int(now.Sub(a.Born).Seconds()), Name: name,
		})
	}
	m.mu.Unlock()
	return out
}

func (m *permManager) pendingCount(sid string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, a := range m.pending {
		if sid == "" || a.SID == sid {
			n++
		}
	}
	return n
}

// Mode 返回一支船队当前的权限模式（默认 auto）。
func (m *permManager) Mode(sid string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return normMode(m.mode[sid])
}

// setMode 设置一支船队的权限模式；auto 即恢复默认（不落多余键）。
func (m *permManager) setMode(sid, mode string) {
	mode = normMode(mode)
	m.mu.Lock()
	if mode == ModeAuto {
		delete(m.mode, sid)
	} else {
		m.mode[sid] = mode
	}
	m.mu.Unlock()
}

// isAutopilot 兼容旧口径：yolo（无人值守）即旧「放手/本会话免审批」。
func (m *permManager) isAutopilot(sid string) bool {
	return m.Mode(sid) == ModeYolo
}

// mcpConfig 是 --mcp-config 文件里我们要写的最小结构。
type mcpConfig struct {
	MCPServers map[string]mcpServer `json:"mcpServers"`
}
type mcpServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// configFor 为某支船队写一份 MCP 配置（env 里带上 socket/令牌/sid），返回路径。
// 每船队一份是为了让 MCP 子进程在请示帧里自带 sid，主进程才能对应到具体船队。
func (m *permManager) configFor(sid string) (string, error) {
	cfg := mcpConfig{MCPServers: map[string]mcpServer{
		"mindport": {
			Command: m.exe,
			Args:    []string{"_perm-mcp"},
			Env: map[string]string{
				"PORT_PERM_SOCK":  m.sockPath,
				"PORT_PERM_TOKEN": m.token,
				"PORT_PERM_FLEET": sid,
			},
		},
	}}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	p := filepath.Join(m.cfgDir, "mcp-"+sanitizeSID(sid)+".json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return "", err
	}
	return p, nil
}

// removeConfig 拆船时清掉配置文件（best effort）。
func (m *permManager) removeConfig(sid string) {
	if m == nil {
		return
	}
	_ = os.Remove(filepath.Join(m.cfgDir, "mcp-"+sanitizeSID(sid)+".json"))
	m.mu.Lock()
	delete(m.mode, sid)
	m.mu.Unlock()
}

func sanitizeSID(sid string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", "..", "_")
	s := r.Replace(sid)
	return s
}

// permSummarize 生成审批卡片的一句话摘要与展开详情。
func permSummarize(tool string, input json.RawMessage) (summary, detail string) {
	var m map[string]any
	if json.Unmarshal(input, &m) != nil {
		return tool, clipText(string(input), 1200)
	}
	str := func(k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	base := func(p string) string { return filepath.Base(p) }
	switch tool {
	case "Bash", "BashOutput":
		cmd := strings.TrimSpace(firstLineLocal(str("command")))
		if cmd == "" {
			cmd = str("command_id")
		}
		return clipText("$ "+cmd, 200), clipText(cmd, 2000)
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		p := str("file_path")
		return base(p), clipText(p, 1200)
	case "Read":
		return "读 " + base(str("file_path")), str("file_path")
	default:
		detail = clipText(string(input), 1200)
		return tool, detail
	}
}

func firstLineLocal(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// subtleNotEqual 是常量时间比较的简化版（本地 unix + 令牌，够⽤且不引额外文件）。
func subtleNotEqual(a, b string) bool {
	if len(a) != len(b) {
		return true
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v != 0
}

// ---------- MCP stdio 子进程：port _perm-mcp ----------

// RunPermMCP 跑一个最小 MCP server（stdio JSON-RPC），把 check_permission
// 工具调用转成 unix socket 请示。独立成函数（不绑 cobra）以便单测。
// dial 可注入；传 nil 用 net.Dial("unix")。
func RunPermMCP(in io.Reader, out io.Writer, getenv func(string) string, dial func(string) (net.Conn, error)) error {
	sock := getenv("PORT_PERM_SOCK")
	token := getenv("PORT_PERM_TOKEN")
	sid := getenv("PORT_PERM_FLEET")
	if sock == "" {
		return fmt.Errorf("PORT_PERM_SOCK 未设置")
	}
	if dial == nil {
		dial = func(addr string) (net.Conn, error) { return net.Dial("unix", addr) }
	}

	var outMu sync.Mutex
	write := func(v any) {
		outMu.Lock()
		defer outMu.Unlock()
		b, _ := json.Marshal(v)
		_, _ = out.Write(append(b, '\n'))
	}

	type rpcIn struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	initialized := false
	var wg sync.WaitGroup // 在途 tools/call：stdin 关闭前把审批收束完

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg rpcIn
		if json.Unmarshal(line, &msg) != nil {
			continue
		}
		hasID := len(msg.ID) > 0 && string(msg.ID) != "null"

		switch msg.Method {
		case "initialize":
			proto := "2024-11-05"
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if json.Unmarshal(msg.Params, &p) == nil && p.ProtocolVersion != "" {
				proto = p.ProtocolVersion
			}
			initialized = true
			if hasID {
				write(map[string]any{"jsonrpc": "2.0", "id": rawID(msg.ID), "result": map[string]any{
					"protocolVersion": proto,
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "mindport", "version": "0.9"},
				}})
			}
		case "notifications/initialized", "initialized":
			// 仅通知，不回
		case "ping":
			if hasID {
				write(map[string]any{"jsonrpc": "2.0", "id": rawID(msg.ID), "result": map[string]any{}})
			}
		case "tools/list":
			if hasID {
				write(map[string]any{"jsonrpc": "2.0", "id": rawID(msg.ID), "result": map[string]any{
					"tools": []map[string]any{{
						"name":        "check_permission",
						"description": "MindPort 手机端人工审批：等待机主在网页上批准或拒绝本次工具调用",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"tool_name":   map[string]string{"type": "string"},
								"tool_use_id": map[string]string{"type": "string"},
								"input":       map[string]string{"type": "object"},
							},
							"required": []string{"tool_name", "input"},
						},
					}},
				}})
			}
		case "tools/call":
			// CLI 可能并发回调多个工具：每次请示独立 goroutine，写出加锁。
			wg.Add(1)
			go func(id json.RawMessage, params json.RawMessage) {
				defer wg.Done()
				var p struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}
				_ = json.Unmarshal(params, &p)
				var args struct {
					ToolName  string          `json:"tool_name"`
					ToolUseID string          `json:"tool_use_id"`
					Input     json.RawMessage `json:"input"`
				}
				_ = json.Unmarshal(p.Arguments, &args)
				dec := askPermission(dial, sock, token, sid, args.ToolName, args.ToolUseID, args.Input)
				text, _ := json.Marshal(dec)
				write(map[string]any{"jsonrpc": "2.0", "id": rawID(id), "result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": string(text)}},
				}})
			}(msg.ID, msg.Params)
		default:
			// server/discover 等探针或未知方法：有 id 就回空 result，别让 CLI 报错。
			if hasID && initialized {
				write(map[string]any{"jsonrpc": "2.0", "id": rawID(msg.ID), "result": map[string]any{}})
			}
		}
	}
	wg.Wait() // 等在途审批（dial/请示）收束，别在 CLI 读应答前提前退出
	return sc.Err()
}

// rawID 把 json.RawMessage 原样塞进响应（id 可能是字符串或数字）。
func rawID(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(id, &v) == nil {
		return v
	}
	return string(id)
}

// askPermission 经 unix socket 请示主进程并阻塞到拿到判定；任何失败都安全拒绝。
func askPermission(dial func(string) (net.Conn, error), sock, token, sid, tool, toolUseID string,
	input json.RawMessage) permDecision {
	safeDeny := permDecision{Behavior: "deny",
		Message: "思港：审批通道连不上港口，已安全拒绝（请确认思港在运行）"}
	c, err := dial(sock)
	if err != nil {
		return safeDeny
	}
	defer c.Close()
	req := permAskReq{Type: "ask", Token: token, SID: sid,
		ToolUseID: toolUseID, ToolName: tool, Input: input}
	b, err := json.Marshal(req)
	if err != nil {
		return safeDeny
	}
	_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := c.Write(append(b, '\n')); err != nil {
		return safeDeny
	}
	// 主进程最坏 5 分钟（+余量）才收束；这里给 6 分半。
	_ = c.SetReadDeadline(time.Now().Add(permAskTimeout + 90*time.Second))
	rb, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return permDecision{Behavior: "deny", Message: "思港：等待手机审批超时，已安全拒绝"}
	}
	var d permDecision
	if json.Unmarshal(rb, &d) != nil || (d.Behavior != "allow" && d.Behavior != "deny") {
		return safeDeny
	}
	return d
}
