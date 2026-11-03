package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bestksl/mindport/internal/store"
)

// heldSID 是合法 UUID（mail 文件只收 UUID 形态的 sid）。
const heldSID = "01234567-89ab-cdef-0123-456789abcdef"

type bufWriteCloser struct{ *bytes.Buffer }

func (bufWriteCloser) Close() error { return nil }

func newHeldTestBridge(t *testing.T) (*fleetBridge, *store.Vault) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	v, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{V: v, broker: newBroker()}
	b := newFleetBridge(s)
	t.Cleanup(func() { b.stopHeld(heldSID) })
	return b, v
}

// 占着席位时 tick 只能等；席位一空就用第一条留言点火起桥，留言原子认领。
func TestHeldTickWaitsThenDelivers(t *testing.T) {
	b, v := newHeldTestBridge(t)
	if _, err := v.AppendMail(heldSID, "第一条"); err != nil {
		t.Fatal(err)
	}
	held := true
	b.testHolder = func(string) bool { return held }
	var started []string
	b.testStart = func(dir, sid, first string) error {
		started = append(started, first)
		return nil
	}

	if done := b.heldTick(heldSID, "/repo/a"); done {
		t.Fatal("席位被占着时 tick 不应收工")
	}
	if len(started) != 0 {
		t.Fatalf("占席期间不应起桥；started=%v", started)
	}

	held = false
	if done := b.heldTick(heldSID, "/repo/a"); !done {
		t.Fatal("席位空出后 tick 应收工")
	}
	if len(started) != 1 || started[0] != "第一条" {
		t.Fatalf("第一条留言应用于点火，got %v", started)
	}
	if n, _ := v.MailPending(heldSID); n != 0 {
		t.Fatalf("投递后留言应全部认领，pending=%d", n)
	}
}

// 多条留言：第一条点火，其余写进新桥 stdin 排队续跑。
func TestHeldTickMultipleMails(t *testing.T) {
	b, v := newHeldTestBridge(t)
	if _, err := v.AppendMail(heldSID, "第一条"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.AppendMail(heldSID, "第二条"); err != nil {
		t.Fatal(err)
	}
	b.testHolder = func(string) bool { return false }

	var sink bytes.Buffer
	var started []string
	b.testStart = func(dir, sid, first string) error {
		started = append(started, first)
		fp := &fleetProc{
			sid: sid, dir: dir, state: bridgeIdle, born: time.Now(),
			procDone: make(chan struct{}),
			stdin:    bufWriteCloser{&sink},
		}
		b.mu.Lock()
		b.procs[sid] = fp
		b.mu.Unlock()
		return nil
	}

	if done := b.heldTick(heldSID, "/repo/a"); !done {
		t.Fatal("席位空闲，tick 一次就应收工")
	}
	if len(started) != 1 || started[0] != "第一条" {
		t.Fatalf("首条点火不符：%v", started)
	}
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(sink.Bytes()), &line); err != nil {
		t.Fatalf("第二条应作为 stream-json user 帧写入 stdin: %v (%q)", err, sink.String())
	}
	msg, _ := line["message"].(map[string]any)
	if msg["content"] != "第二条" {
		t.Fatalf("stdin 里应是第二条，got %v", msg["content"])
	}
	if n, _ := v.MailPending(heldSID); n != 0 {
		t.Fatalf("全部留言应认领，pending=%d", n)
	}
}

// mail-pull 钩子先把留言拉走：watcher 静默收工，绝不起桥重复注入。
func TestHeldTickHookPulled(t *testing.T) {
	b, v := newHeldTestBridge(t)
	if _, err := v.AppendMail(heldSID, "交给钩子吧"); err != nil {
		t.Fatal(err)
	}
	if mails, err := v.PullMail(heldSID); err != nil || len(mails) != 1 {
		t.Fatalf("钩子认领失败: %v %d", err, len(mails))
	}
	called := false
	b.testStart = func(dir, sid, first string) error { called = true; return nil }

	if done := b.heldTick(heldSID, "/repo/a"); !done {
		t.Fatal("没有待投递留言应立即收工")
	}
	if called {
		t.Fatal("留言已被钩子拉走，不应再起火")
	}
}

// 起桥时席位又被抢回（Start 返回 held）：留言不认领，下轮再试。
func TestHeldTickStartRaces(t *testing.T) {
	b, v := newHeldTestBridge(t)
	if _, err := v.AppendMail(heldSID, "竞速"); err != nil {
		t.Fatal(err)
	}
	// 探测口说空了，但 Start 内部再探时又被占。
	b.testHolder = func(string) bool { return false }
	b.testStart = func(dir, sid, first string) error { return errSessionHeldElsewhere }

	if done := b.heldTick(heldSID, "/repo/a"); done {
		t.Fatal("起桥竞态失败时 watcher 应继续等")
	}
	if n, _ := v.MailPending(heldSID); n != 1 {
		t.Fatalf("竞态失败留言必须保留，pending=%d", n)
	}
}

// holdWatch 幂等 + watcher 真能在席位空出后自动投递（异步链路）。
func TestHeldWatchAsync(t *testing.T) {
	old := heldProbeInterval
	heldProbeInterval = 20 * time.Millisecond
	t.Cleanup(func() { heldProbeInterval = old })

	b, v := newHeldTestBridge(t)
	if _, err := v.AppendMail(heldSID, "异步排队"); err != nil {
		t.Fatal(err)
	}
	held := true
	b.testHolder = func(string) bool { return held }
	got := make(chan string, 1)
	b.testStart = func(dir, sid, first string) error {
		select {
		case got <- first:
		default:
		}
		return nil
	}

	b.holdWatch(heldSID, "/repo/a")
	b.holdWatch(heldSID, "/repo/a") // 幂等：不重复起
	if !b.heldWatching(heldSID) {
		t.Fatal("holdWatch 后应在等席位")
	}
	time.Sleep(60 * time.Millisecond)
	select {
	case <-got:
		t.Fatal("占席期间不应起桥")
	default:
	}

	held = false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := v.MailPending(heldSID); n == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n, _ := v.MailPending(heldSID); n != 0 {
		t.Fatalf("席位空出后 watcher 应自动投递，pending=%d", n)
	}
	select {
	case first := <-got:
		if first != "异步排队" {
			t.Fatalf("点火留言不符：%q", first)
		}
	default:
		t.Fatal("watcher 没起桥")
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !b.heldWatching(heldSID) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("投递完成后 watcher 应注销")
}

// Kill 撤掉 watcher（删除会话不再等席位）。
func TestHeldStopOnKill(t *testing.T) {
	b, v := newHeldTestBridge(t)
	if _, err := v.AppendMail(heldSID, "别等了"); err != nil {
		t.Fatal(err)
	}
	b.testHolder = func(string) bool { return true }
	b.holdWatch(heldSID, "/repo/a")
	if !b.heldWatching(heldSID) {
		t.Fatal("应已挂上 watcher")
	}
	b.Kill(heldSID)
	if b.heldWatching(heldSID) {
		t.Fatal("Kill 后 watcher 应撤销")
	}
}

// serve 重启：有未投递留言、没有桥的会话自动重新挂上 watcher。
func TestResumeHeldWatches(t *testing.T) {
	b, v := newHeldTestBridge(t)
	if err := v.SaveFleet(store.FleetInfo{SID: heldSID, Dir: "/repo/a", Name: "登录"}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.AppendMail(heldSID, "重启前排队"); err != nil {
		t.Fatal(err)
	}
	old := heldProbeInterval
	heldProbeInterval = 20 * time.Millisecond
	t.Cleanup(func() { heldProbeInterval = old })
	b.testHolder = func(string) bool { return true }

	b.resumeHeldWatches()
	if !b.heldWatching(heldSID) {
		t.Fatal("重启后应恢复 watcher")
	}
}

// HTTP 全链路：会话被占着时 say 不返回 409，而是 held + 排队 + watcher 在岗。
func TestSayHeldQueuesInsteadOf409(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	v, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.PresentSession(store.Presence{
		Actor: "claude@studio", SessionID: heldSID, Dir: testCWD, Name: "占席会话",
	}); err != nil {
		t.Fatal(err)
	}
	projDir := filepath.Join(home, ".claude", "projects", "-repo-a")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, heldSID+".jsonl"), []byte(testTranscript), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{V: v, broker: newBroker()}
	s.poller = &agentPoller{actor: "claude@studio", v: v, exe: "/bin/echo"}
	b := newFleetBridge(s)
	s.bridge = b // Routes() 走的是 s.bridge
	t.Cleanup(func() { b.stopHeld(heldSID) })
	b.testHolder = func(string) bool { return true }

	form := url.Values{"msg": {"我就说一句，不许拒绝我"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sessions/"+heldSID+"/say?dir="+url.QueryEscape(testCWD),
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("占席不应再报 409，got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"delivered":"held"`) {
		t.Fatalf("应答 delivered=held：%s", body)
	}
	if n, _ := v.MailPending(heldSID); n != 1 {
		t.Fatalf("留言应落盘排队，pending=%d", n)
	}
	if !b.heldWatching(heldSID) {
		t.Fatal("应已挂上席位 watcher")
	}
}
