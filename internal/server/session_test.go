package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bestksl/mindport/internal/store"
)

const (
	testSID = "sess-testa-0001"
	testCWD = "/repo/a"
)

// setupSession 造一个带磁盘心跳与 transcript 文件的会话；home 与 vault 各自临时目录。
func setupSession(t *testing.T, transcriptLines string, live map[string]cliSession) (*Server, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	v, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.PresentSession(store.Presence{
		Actor: "claude@studio", SessionID: testSID, Dir: testCWD, Name: "登录改造",
	}); err != nil {
		t.Fatal(err)
	}
	// 复刻目录编码：非字母数字 → '-'
	projDir := filepath.Join(home, ".claude", "projects", "-repo-a")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if transcriptLines != "" {
		if err := os.WriteFile(filepath.Join(projDir, testSID+".jsonl"),
			[]byte(transcriptLines), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{V: v, broker: newBroker()}
	if live != nil {
		s.poller = &agentPoller{actor: "claude@studio", v: v, live: live}
	}
	return s, home
}

const testTranscript = `{"type":"user","uuid":"u1","timestamp":"2026-10-26T10:00:00.000Z","message":{"role":"user","content":"先把登录做了"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-26T10:00:01.000Z","message":{"role":"assistant","content":[{"type":"text","text":"好，我先加登录页。"},{"type":"tool_use","id":"tu1","name":"Edit","input":{"file_path":"/repo/auth.go"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-26T10:00:02.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"done"}]}}
`

func TestSessionPageRendersTail(t *testing.T) {
	s, _ := setupSession(t, testTranscript, nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/sessions/"+testSID, nil))
	if rec.Code != 200 {
		t.Fatalf("页面应 200，得到 %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"先把登录做了", "好，我先加登录页。", "登录改造", "Edit"} {
		if !strings.Contains(body, want) {
			t.Fatalf("首屏缺内容 %q", want)
		}
	}
}

func TestSessionStreamIdempotent(t *testing.T) {
	s, _ := setupSession(t, testTranscript, nil)
	h := s.Routes()

	get := func(target string) streamResp {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		var d streamResp
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
			t.Fatalf("解析失败: %v body=%s", err, rec.Body.String())
		}
		return d
	}
	first := get("/api/sessions/" + testSID + "/stream?from=0")
	// tool_result 行折叠进同 tid 的工具卡：u1 + a1 两事件，r1 不单列
	if len(first.Events) != 2 || first.Cursor != first.Size {
		t.Fatalf("首次增量应得 2 事件且游标到尾: %+v", first)
	}
	var toolDone bool
	for _, b := range first.Events[1].Blocks {
		if b.Kind == "tool" && b.ToolID == "tu1" && b.Result == "done" {
			toolDone = true
		}
	}
	if !toolDone {
		t.Fatalf("工具卡应并入结果 done: %+v", first.Events[1].Blocks)
	}
	// 同游标重复：空事件、游标不动，且 wire 上必须是 [] 而非 null（前端直接 .length）
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET",
		"/api/sessions/"+testSID+"/stream?from="+strconv.FormatInt(first.Cursor, 10), nil))
	if !strings.Contains(rec.Body.String(), `"events":[]`) {
		t.Fatalf("空增量 wire 应含 \"events\":[]，得到 %s", rec.Body.String())
	}
	var again streamResp
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if len(again.Events) != 0 || again.Cursor != first.Cursor {
		t.Fatalf("重复增量应为空且游标不动: %+v", again)
	}
}

func TestSessionHistoryAnd404(t *testing.T) {
	s, _ := setupSession(t, testTranscript, nil)
	h := s.Routes()

	// 上翻：小文件从头读，from=0（到会话开端）
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions/"+testSID+"/history?before=999999", nil))
	var d historyResp
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Events) != 2 || d.From != 0 {
		t.Fatalf("上翻应得 2 事件（工具回报并入工具卡）且 From=0: %+v", d)
	}
	var sawResult bool
	for _, b := range d.Events[1].Blocks {
		if b.Kind == "tool" && b.Result == "done" {
			sawResult = true
		}
	}
	if !sawResult {
		t.Fatalf("历史窗口里工具卡也应带结果: %+v", d.Events[1].Blocks)
	}

	// 心跳里没有的会话：页面友好提示
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions/nope-0000", nil))
	if !strings.Contains(rec.Body.String(), "没查到这支船队") {
		t.Fatalf("未知会话应显示友好态")
	}
	// API 直接 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions/nope-0000/stream", nil))
	if rec.Code != 404 {
		t.Fatalf("未知会话 stream 应 404，得到 %d", rec.Code)
	}
	// 非法 sid（含点）404，不碰文件系统
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions/evil.sid/stream", nil))
	if rec.Code != 404 {
		t.Fatalf("非法 sid 应 404，得到 %d", rec.Code)
	}
}

func TestSessionSayBusyGoesToMailbox(t *testing.T) {
	live := map[string]cliSession{
		testSID: {SessionID: testSID, CWD: testCWD, Status: "busy", State: "working"},
	}
	s, _ := setupSession(t, testTranscript, live)
	h := s.Routes()

	form := url.Values{"msg": {"先别停，顺手把鉴权也做了"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sessions/"+testSID+"/say", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	var d sayResp
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Delivered != "mail" || d.Pending != 1 {
		t.Fatalf("忙碌会话应进信箱，得到 %+v", d)
	}
	got, err := s.V.PullMail(testSID)
	if err != nil || len(got) != 1 || got[0].Text != "先别停，顺手把鉴权也做了" {
		t.Fatalf("钩子应能拉到留言: %+v err=%v", got, err)
	}
}

func TestSessionSayResumeFailureFallsBack(t *testing.T) {
	// 离线会话（live 缺席）→ 尝试 resume；但 PATH 无 claude 且 HOME 是临时目录，
	// 找不到 CLI 时必须兜底进信箱，指令不丢。
	t.Setenv("PATH", t.TempDir())
	s, _ := setupSession(t, testTranscript, nil)
	h := s.Routes()

	form := url.Values{"msg": {"回来再补一页 ADR"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sessions/"+testSID+"/say", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	var d sayResp
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Delivered != "mail" {
		t.Fatalf("resume 不可用时应兜底信箱，得到 %+v", d)
	}

	// 空消息 400、未知会话 404
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/sessions/"+testSID+"/say",
		strings.NewReader(url.Values{"msg": {"  "}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("空消息应 400，得到 %d", rec.Code)
	}
}

// 2.1.310 的 --bg 回执是真人横幅（即便要求 JSON）：短 id 必须抓得住。
func TestBackgroundAckParse(t *testing.T) {
	banner := []byte(`backgrounded · c8e5a536
  claude agents             list sessions
  claude attach c8e5a536    open in this terminal
  claude stop c8e5a536      stop this session
`)
	m := bgAckRe.FindSubmatch(banner)
	if m == nil || string(m[1]) != "c8e5a536" {
		t.Fatalf("没抓到短 id：%v", m)
	}
	// 未来版本若回 JSON，UUID 提取也要中
	j := []byte(`{"sessionId":"c8e5a536-d16d-4546-a6f4-124c420e2b55","state":"backgrounded"}`)
	if sid := extractSessionID(j); sid != "c8e5a536-d16d-4546-a6f4-124c420e2b55" {
		t.Fatalf("JSON 回执 UUID 提取失败：%q", sid)
	}
	// 抓不到不允许瞎猜
	if bgAckRe.MatchString("Workspace not trusted. Run `claude` in /x once.") {
		t.Fatalf("错误横幅里不应匹配出短 id")
	}
}
