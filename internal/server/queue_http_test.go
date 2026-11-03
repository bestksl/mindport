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

	"github.com/bestksl/mindport/internal/store"
)

// 排队消息的撤回/编辑走 HTTP 全链路：返回最新队列快照，供前端重渲染虚线卡。
func TestQueueRemoveEditHTTP(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	v, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.PresentSession(store.Presence{
		Actor: "claude@studio", SessionID: heldSID, Dir: testCWD, Name: "队列会话",
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
	s.bridge = b
	t.Cleanup(func() { b.stopHeld(heldSID) })

	var sink bytes.Buffer
	fp := &fleetProc{
		sid: heldSID, dir: testCWD, state: bridgeWorking, gen: 1,
		stdin: bufWriteCloser{&sink}, procDone: make(chan struct{}),
	}
	b.procs[heldSID] = fp
	fp.queued = []string{"第一句", "第二句", "第三句"}

	post := func(path string, form url.Values) (int, queueResp) {
		req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, req)
		var qr queueResp
		if rec.Code == 200 {
			if err := json.Unmarshal(rec.Body.Bytes(), &qr); err != nil {
				t.Fatalf("应答不是合法 JSON: %v (%s)", err, rec.Body.String())
			}
		}
		return rec.Code, qr
	}
	dirQ := url.QueryEscape(testCWD)

	code, qr := post("/sessions/"+heldSID+"/queue/remove?dir="+dirQ,
		url.Values{"idx": {"1"}})
	if code != 200 {
		t.Fatalf("撤回应 200，got %d", code)
	}
	if len(qr.Queued) != 2 || qr.Queued[0] != "第一句" || qr.Queued[1] != "第三句" {
		t.Fatalf("撤回后队列快照不符: %+v", qr.Queued)
	}
	if sink.Len() != 0 {
		t.Fatal("撤回排队句绝不能写 stdin")
	}

	code, qr = post("/sessions/"+heldSID+"/queue/edit?dir="+dirQ,
		url.Values{"idx": {"1"}, "msg": {"  改成这样 "}})
	if code != 200 {
		t.Fatalf("编辑应 200，got %d", code)
	}
	if len(qr.Queued) != 2 || qr.Queued[1] != "改成这样" {
		t.Fatalf("编辑后队列快照不符: %+v", qr.Queued)
	}

	code, _ = post("/sessions/"+heldSID+"/queue/remove?dir="+dirQ,
		url.Values{"idx": {"9"}})
	if code != 410 {
		t.Fatalf("越界撤回应 410，got %d", code)
	}
	code, _ = post("/sessions/"+heldSID+"/queue/edit?dir="+dirQ,
		url.Values{"idx": {"0"}, "msg": {"   "}})
	if code != 400 {
		t.Fatalf("改成空话应 400，got %d", code)
	}
}
