package server

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bestksl/mindport/internal/store"
)

// safeBuf 是供多 goroutine（并发 tools/call 应答）写入的线程安全缓冲。
type safeBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestRunPermMCPProtocol 验证 stdio MCP 子进程：握手/工具表/tools-call 经
// 注入的假 unix 连接请示，并把主进程的 allow|deny 判定原样回给 CLI。
func TestRunPermMCPProtocol(t *testing.T) {
	pr, pw := io.Pipe()
	var out safeBuf

	// 每次请示新建一对 net.Pipe：另一端是假「serve 主进程」，按工具名给判定。
	dial := func(addr string) (net.Conn, error) {
		a, b := net.Pipe()
		go func() {
			defer b.Close()
			br := bufio.NewReader(b)
			line, err := br.ReadBytes('\n')
			if err != nil {
				return
			}
			var req permAskReq
			if json.Unmarshal(line, &req) != nil {
				return
			}
			dec := permDecision{Behavior: "allow"}
			if req.ToolName == "Bash" {
				dec = permDecision{Behavior: "deny", Message: "no bash"}
			}
			jb, _ := json.Marshal(dec)
			_, _ = b.Write(append(jb, '\n'))
		}()
		return a, nil
	}
	env := func(k string) string {
		switch k {
		case "PORT_PERM_SOCK":
			return "/tmp/fake.sock"
		case "PORT_PERM_TOKEN":
			return "tok"
		case "PORT_PERM_FLEET":
			return "sid-1"
		}
		return ""
	}

	runErr := make(chan error, 1)
	go func() { runErr <- RunPermMCP(pr, &out, env, dial) }()

	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = pw.Write(append(b, '\n'))
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2024-11-05"}})
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	send(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{
		"name": "check_permission",
		"arguments": map[string]any{
			"tool_name": "Read", "tool_use_id": "c1",
			"input": map[string]any{"file_path": "/tmp/a"},
		},
	}})
	send(map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{
		"name": "check_permission",
		"arguments": map[string]any{
			"tool_name": "Bash", "tool_use_id": "c2",
			"input": map[string]any{"command": "rm -rf /"},
		},
	}})
	_ = pw.Close()

	select {
	case err := <-runErr:
		if err != nil && err != io.EOF {
			t.Fatalf("RunPermMCP: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunPermMCP 没在 EOF 后收束")
	}

	got := out.String()
	if !strings.Contains(got, "check_permission") {
		t.Error("tools/list 没暴露 check_permission")
	}
	// 判定 JSON 嵌在工具返回的 text 字符串里，引号被转义成 \"
	if !strings.Contains(got, `behavior\":\"allow`) {
		t.Error("Read 请示应透传 allow；输出：", got)
	}
	if !strings.Contains(got, `behavior\":\"deny`) || !strings.Contains(got, "no bash") {
		t.Error("Bash 请示应透传 deny 与理由；输出：", got)
	}
}

// TestPermManagerFlow 验证主进程：只读工具自动放行、危险工具挂起待批、
// 手机拍板放行、autopilot 豁免。
func TestPermManagerFlow(t *testing.T) {
	v, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := New(v)
	if err := s.EnablePerms(); err != nil {
		t.Fatalf("EnablePerms: %v", err)
	}
	m := s.perm
	defer m.ln.Close()

	sid := "11111111-1111-4111-8111-111111111111"
	ask := func(tool, id string, input map[string]any) (net.Conn, chan string) {
		c, err := net.Dial("unix", m.sockPath)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		in, _ := json.Marshal(input)
		req := permAskReq{Type: "ask", Token: m.token, SID: sid,
			ToolUseID: id, ToolName: tool, Input: in}
		jb, _ := json.Marshal(req)
		if _, err := c.Write(append(jb, '\n')); err != nil {
			t.Fatalf("write ask: %v", err)
		}
		ch := make(chan string, 1)
		go func() {
			bs, err := bufio.NewReader(c).ReadString('\n')
			if err == nil {
				ch <- strings.TrimSpace(bs)
			}
			close(ch)
		}()
		return c, ch
	}

	// 1) Read 只读：立即 allow，不进 pending。
	_, readCh := ask("Read", "r1", map[string]any{"file_path": "/tmp/a"})
	select {
	case got := <-readCh:
		if !strings.Contains(got, `"behavior":"allow"`) {
			t.Fatalf("只读工具应自动 allow，得到 %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("只读工具没有被自动放行")
	}
	if n := m.pendingCount(sid); n != 0 {
		t.Fatalf("只读工具不应挂起，pending=%d", n)
	}

	// 2) Bash 危险：挂起，可在 views 看到。
	cb, bashCh := ask("Bash", "b1", map[string]any{"command": "echo hi"})
	defer cb.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && m.pendingCount(sid) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	views := m.views(sid)
	if len(views) != 1 || views[0].Tool != "Bash" {
		t.Fatalf("应看到 1 条 Bash 待批，得到 %+v", views)
	}
	if !strings.Contains(views[0].Summary, "echo hi") {
		t.Errorf("摘要应含命令，得到 %q", views[0].Summary)
	}

	// 3) 手机批准：连接收到 allow。
	if !m.decide("b1", "allow") {
		t.Fatal("decide 没找到待批项")
	}
	select {
	case got := <-bashCh:
		if !strings.Contains(got, `"behavior":"allow"`) {
			t.Fatalf("批准后应 allow，得到 %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("批准后子进程没收到判定")
	}

	// 4) yolo（无人值守，旧 autopilot/放手）：危险工具也直接放行。
	m.setMode(sid, ModeYolo)
	if !m.isAutopilot(sid) {
		t.Fatal("yolo 模式 isAutopilot 应为真")
	}
	if m.Mode(sid) != ModeYolo {
		t.Fatal("模式应回读为 yolo")
	}
	// 默认/非法模式归一化为 auto
	m.setMode(sid, "乱七八糟")
	if m.Mode(sid) != ModeAuto {
		t.Fatalf("非法模式应归一化为 auto，得到 %q", m.Mode(sid))
	}
	m.setMode(sid, ModeYolo)
	_, autoCh := ask("Bash", "b2", map[string]any{"command": "ls"})
	select {
	case got := <-autoCh:
		if !strings.Contains(got, `"behavior":"allow"`) {
			t.Fatalf("autopilot 下应直接 allow，得到 %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("autopilot 下仍被挂起")
	}
}

func TestPermSummarize(t *testing.T) {
	sum, _ := permSummarize("Bash", json.RawMessage(`{"command":"rm -rf build\nother"}`))
	if !strings.HasPrefix(sum, "$ rm -rf build") {
		t.Errorf("Bash 摘要异常: %q", sum)
	}
	sum, _ = permSummarize("Write", json.RawMessage(`{"file_path":"/a/b/note.txt"}`))
	if sum != "note.txt" {
		t.Errorf("Write 摘要应为文件名，得到 %q", sum)
	}
}
