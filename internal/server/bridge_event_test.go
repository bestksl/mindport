package server

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// runReadLoop 把若干 stream-json 行喂给指定代数的 readLoop，喂完即 EOF。
func runReadLoop(b *fleetBridge, fp *fleetProc, gen uint64, lines ...string) {
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	b.readLoop(fp, strings.NewReader(sb.String()), gen)
}

// 逐字直播：thinking/text/tool 增量事件必须折叠成当前轮流式块。
func TestReadLoopStreamFolding(t *testing.T) {
	b := newFleetBridge(nil)
	fp := &fleetProc{sid: "s1", state: bridgeStarting, gen: 1}
	b.procs["s1"] = fp

	runReadLoop(b, fp, 1,
		`{"type":"system","subtype":"init"}`,
		`{"type":"system","subtype":"status","status":"requesting"}`,
		`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"嗯，"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"算一下"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"text"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"2"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","name":"Bash"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"c":"}}}`,
	)

	fp.mu.Lock()
	defer fp.mu.Unlock()
	if fp.state != bridgeWorking {
		t.Fatalf("requesting 后应 working，实得 %s", fp.state)
	}
	if len(fp.blocks) != 3 {
		t.Fatalf("应折出 3 个块，实得 %d: %+v", len(fp.blocks), fp.blocks)
	}
	if fp.blocks[0].Kind != "thinking" || fp.blocks[0].Text != "嗯，算一下" {
		t.Errorf("thinking 折叠错误: %+v", fp.blocks[0])
	}
	if fp.blocks[1].Kind != "text" || fp.blocks[1].Text != "2" {
		t.Errorf("text 折叠错误: %+v", fp.blocks[1])
	}
	if fp.blocks[2].Kind != "tool" || fp.blocks[2].Name != "Bash" {
		t.Errorf("tool 块错误: %+v", fp.blocks[2])
	}
}

// result 收束：回 idle、清流式块、弹队首。
func TestReadLoopResultSettles(t *testing.T) {
	b := newFleetBridge(nil)
	fp := &fleetProc{
		sid: "s2", state: bridgeWorking, gen: 1,
		blocks:   []partialBlock{{Kind: "text", Text: "半截"}},
		queued:   []string{"第一问", "第二问"},
		stdin:    nopCloserWriter{},
		procDone: make(chan struct{}),
	}
	b.procs["s2"] = fp

	runReadLoop(b, fp, 1,
		`{"type":"result","subtype":"success","is_error":false}`,
	)

	fp.mu.Lock()
	defer fp.mu.Unlock()
	if fp.state != bridgeIdle {
		t.Fatalf("result 后应 idle，实得 %s", fp.state)
	}
	if len(fp.blocks) != 0 {
		t.Errorf("result 后流式块应清空，实得 %+v", fp.blocks)
	}
	if len(fp.queued) != 1 || fp.queued[0] != "第二问" {
		t.Errorf("result 应弹队首，实得 %+v", fp.queued)
	}
}

// is_error 的 result 也要收束到 idle，并留下错误痕迹。
func TestReadLoopResultError(t *testing.T) {
	b := newFleetBridge(nil)
	fp := &fleetProc{sid: "s3", state: bridgeWorking, gen: 1}
	b.procs["s3"] = fp

	runReadLoop(b, fp, 1,
		`{"type":"result","subtype":"error","is_error":true}`,
	)

	fp.mu.Lock()
	defer fp.mu.Unlock()
	if fp.state != bridgeIdle {
		t.Fatalf("错误 result 也应收束到 idle，实得 %s", fp.state)
	}
	if fp.errMsg == "" {
		t.Error("is_error result 应留下 errMsg")
	}
}

// waiting 状态：CLI 报含 wait 的 status 时亮黄灯。
func TestReadLoopWaiting(t *testing.T) {
	b := newFleetBridge(nil)
	fp := &fleetProc{sid: "s4", state: bridgeWorking, gen: 1}
	b.procs["s4"] = fp

	runReadLoop(b, fp, 1,
		`{"type":"system","subtype":"status","status":"waiting_for_input"}`,
	)
	if got := fp.curGenState(); got != bridgeWaiting {
		t.Fatalf("应翻黄灯 waiting，实得 %s", got)
	}
	if status, online := b.bridgeStatus("s4"); !online || status != "waiting" {
		t.Errorf("bridgeStatus 应给 waiting/online，实得 %q/%v", status, online)
	}
}

// 代数隔离：换代后旧代 readLoop 的事件不得污染新代状态。
func TestReadLoopStaleGenIgnored(t *testing.T) {
	b := newFleetBridge(nil)
	fp := &fleetProc{sid: "s5", state: bridgeCold}
	b.procs["s5"] = fp
	fp.gen = 2 // 当前已是第二代

	runReadLoop(b, fp, 1, // 旧代 goroutine 迟到的事件
		`{"type":"system","subtype":"status","status":"requesting"}`,
		`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text"}}}`,
	)

	fp.mu.Lock()
	defer fp.mu.Unlock()
	if fp.state != bridgeCold {
		t.Errorf("旧代事件不得改状态，实得 %s", fp.state)
	}
	if len(fp.blocks) != 0 {
		t.Errorf("旧代流式块不得写入，实得 %+v", fp.blocks)
	}
}

// 回归：pumpQueue 空队列/no-op、正常投递弹队首、写失败放回队首不丢话。
func TestPumpQueue(t *testing.T) {
	b := newFleetBridge(nil)
	fp := &fleetProc{sid: "s6", state: bridgeIdle, gen: 1, stdin: nopCloserWriter{}, procDone: make(chan struct{})}
	b.procs["s6"] = fp
	b.pumpQueue(fp) // 空队列不应 panic

	fp.queued = []string{"只有一句"}
	b.pumpQueue(fp)
	if len(fp.queued) != 0 {
		t.Fatalf("队头应已投出，剩 %v", fp.queued)
	}

	bad := &fleetProc{sid: "s7", state: bridgeIdle, gen: 1, stdin: errNopCloser{}, procDone: make(chan struct{})}
	b.procs["s7"] = bad
	bad.queued = []string{"写不进去", "第二"}
	b.pumpQueue(bad)
	if len(bad.queued) != 2 || bad.queued[0] != "写不进去" {
		t.Fatalf("投递失败应原样放回队首，got %v", bad.queued)
	}
}

// 服务端队列逐条撤回/编辑（对标 t3 queued-run.cancel/edit）。
func TestQueuedRemoveEdit(t *testing.T) {
	b := newFleetBridge(nil)
	fp := &fleetProc{sid: "s8", state: bridgeWorking, gen: 1, stdin: nopCloserWriter{}, procDone: make(chan struct{})}
	b.procs["s8"] = fp
	fp.queued = []string{"一", "二", "三"}

	got, err := b.RemoveQueued("s8", 1)
	if err != nil || got != "二" || len(fp.queued) != 2 || fp.queued[1] != "三" {
		t.Fatalf("撤回队中句失败: %q %v %v", got, err, fp.queued)
	}
	if err := b.EditQueued("s8", 1, " 改好的三 "); err != nil || fp.queued[1] != "改好的三" {
		t.Fatalf("编辑排队句失败: %v %v", err, fp.queued)
	}
	if _, err := b.RemoveQueued("s8", 9); err == nil {
		t.Fatal("越界撤回应报错")
	}
	if err := b.EditQueued("s8", 0, "  "); err == nil {
		t.Fatal("改成空话应报错")
	}
	if _, err := b.RemoveQueued("nope", 0); err == nil {
		t.Fatal("没桥应报错")
	}
}

// waiting（AskUserQuestion 等人回答）时发话必须直写 stdin 放行当前轮，
// 绝不能进服务端队列——队列要等 result 才投递，会死锁。
func TestSayWhileWaitingDeliversImmediately(t *testing.T) {
	b := newFleetBridge(nil)
	var sink bytes.Buffer
	fp := &fleetProc{
		sid: "s9", state: bridgeWaiting, gen: 1,
		stdin: bufWriteCloser{&sink}, procDone: make(chan struct{}),
		queued: []string{"忙时排的一句"},
	}
	b.procs["s9"] = fp

	queued, err := b.Say("s9", "这是我的答案")
	if err != nil || queued {
		t.Fatalf("waiting 时应直写不排队: queued=%v err=%v", queued, err)
	}
	if len(fp.queued) != 1 {
		t.Fatalf("原排队内容不应动，got %v", fp.queued)
	}
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(sink.Bytes()), &line); err != nil {
		t.Fatalf("stdin 应有一帧 user: %v (%q)", err, sink.String())
	}
	msg, _ := line["message"].(map[string]any)
	if msg["content"] != "这是我的答案" {
		t.Fatalf("写进 stdin 的答案不符: %v", msg["content"])
	}
}

// 忙时发话只进服务端队列、不碰 stdin：排队句因此还能撤回/编辑。
func TestSayWhileBusyEnqueues(t *testing.T) {
	b := newFleetBridge(nil)
	var sink bytes.Buffer
	fp := &fleetProc{
		sid: "s10", state: bridgeWorking, gen: 1,
		stdin: bufWriteCloser{&sink}, procDone: make(chan struct{}),
	}
	b.procs["s10"] = fp

	queued, err := b.Say("s10", "插话")
	if err != nil || !queued {
		t.Fatalf("忙时应入队: queued=%v err=%v", queued, err)
	}
	if sink.Len() != 0 {
		t.Fatal("忙时不得写 stdin")
	}
	if len(fp.queued) != 1 || fp.queued[0] != "插话" {
		t.Fatalf("队列不符: %v", fp.queued)
	}
}

type nopCloserWriter struct{}

func (nopCloserWriter) Write(p []byte) (int, error) { return len(p), nil }
func (nopCloserWriter) Close() error                { return nil }

type errNopCloser struct{}

func (errNopCloser) Write(p []byte) (int, error) { return 0, io.ErrClosedPipe }
func (errNopCloser) Close() error                { return nil }

// 空闲退役：超时 idle/cold 桥置 dead 但不杀复活资格（killed=false），
// 年轻空闲桥与正忙/等人的桥不动。
func TestReapIdleRetires(t *testing.T) {
	b := newFleetBridge(nil)
	mk := func(sid, state string, idleAt time.Time) *fleetProc {
		fp := &fleetProc{sid: sid, state: state, gen: 1, idleAt: idleAt}
		b.procs[sid] = fp
		return fp
	}
	old := mk("old", bridgeIdle, time.Now().Add(-2*time.Minute))
	cold := mk("cold", bridgeCold, time.Now().Add(-90*time.Second))
	young := mk("young", bridgeIdle, time.Now())
	busy := mk("busy", bridgeWorking, time.Now().Add(-10*time.Minute))
	wait := mk("wait", bridgeWaiting, time.Now().Add(-10*time.Minute))
	gone := mk("gone", bridgeDead, time.Now().Add(-10*time.Minute))

	b.reapOnce()

	if old.state != bridgeDead || old.gen != 2 || old.killed {
		t.Fatalf("超时 idle 桥应退役（dead/gen+1/killed=false），实得 state=%s gen=%d killed=%v",
			old.state, old.gen, old.killed)
	}
	if cold.state != bridgeDead {
		t.Fatalf("超时 cold 空船也应退役，实得 %s", cold.state)
	}
	if young.state != bridgeIdle {
		t.Errorf("未满 TTL 的空闲桥不应动，实得 %s", young.state)
	}
	if busy.state != bridgeWorking {
		t.Errorf("工作中的桥不能退役，实得 %s", busy.state)
	}
	if wait.state != bridgeWaiting {
		t.Errorf("等人拍板的桥不能退役，实得 %s", wait.state)
	}
	if gone.state != bridgeDead {
		t.Errorf("已死桥无需变化，实得 %s", gone.state)
	}
}

// 起桥前席位探测的命令行识别：--resume/--session-id 两种写法都要认。
func TestResumeSidRe(t *testing.T) {
	cases := map[string]string{
		"/x/claude -p --resume abcdef12-3456-7890-abcd-ef1234567890 --verbose": "abcdef12-3456-7890-abcd-ef1234567890",
		"/x/claude --session-id=abcdef12-3456-7890-abcd-ef1234567890":          "abcdef12-3456-7890-abcd-ef1234567890",
		"/x/claude --session-id abcdef12-3456-7890-abcd-ef1234567890 -p":       "abcdef12-3456-7890-abcd-ef1234567890",
		`vim /notes/abcdef12-3456-7890-abcd-ef1234567890.md`:                   "",
		"claude": "",
	}
	for args, want := range cases {
		m := resumeSidRe.FindStringSubmatch(args)
		got := ""
		if m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("args=%q 识别得 %q，应得 %q", args, got, want)
		}
	}
}

// restarting 对外三态映射与列表分组。
func TestRestartingMapping(t *testing.T) {
	if fs := fleetStateOf(true, "restarting"); fs.Key != "restarting" || fs.Text != "连接中" {
		t.Errorf("fleetStateOf restarting 映射错误: %+v", fs)
	}
	mkRow := func(sid, status string, online bool) dirRow {
		var r dirRow
		r.SID = sid
		r.Status = status
		r.Online = online
		return r
	}
	rows := []dirRow{
		mkRow("a", "restarting", true),
		mkRow("b", "busy", true),
		mkRow("c", "idle", true),
	}
	g := groupFleetRows(rows)
	if len(g["working"]) != 2 {
		t.Errorf("restarting 应归工作中组，working 组实得 %d 行", len(g["working"]))
	}
}
