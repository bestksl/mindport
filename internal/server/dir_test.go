package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bestksl/mindport/internal/transcript"
)

// 简单粗暴扫盘：不依赖在线状态/发现口，?dir= 绝对路径直接翻全部会话，
// 早已离线（无在线状态）的会话也能进会话页、读实时流接口。
func TestSessionListByDir(t *testing.T) {
	s, home := setupSession(t, testTranscript, nil)
	h := s.Routes()

	// 列表针对真实存在的项目目录（handler 会 stat 目录本体）
	proj := t.TempDir()
	pdir := transcript.ProjectDir(home, proj)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, testSID+".jsonl"), []byte(testTranscript), 0o600); err != nil {
		t.Fatal(err)
	}

	// 再造一个「无在线状态」历史会话：只有 transcript 文件
	const oldSID = "sess-hist-00002"
	oldBody := `{"type":"user","uuid":"h1","timestamp":"2026-10-20T09:00:00.000Z","message":{"role":"user","content":"历史会话也能打开"}}
{"type":"assistant","uuid":"h2","timestamp":"2026-10-20T09:00:01.000Z","message":{"role":"assistant","content":[{"type":"text","text":"在的。"}]}}
`
	if err := os.WriteFile(filepath.Join(pdir, oldSID+".jsonl"), []byte(oldBody), 0o600); err != nil {
		t.Fatal(err)
	}
	enc := url.QueryEscape(proj)

	// 无参数：空态页带目录输入框 + 顶栏明面入口
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions", nil))
	emptyBody := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(emptyBody, `name="dir"`) {
		t.Fatalf("空态应 200 且带目录输入框，code=%d", rec.Code)
	}
	if !strings.Contains(emptyBody, `class="fleet-btn"`) {
		t.Fatalf("顶栏应有明面的会话入口")
	}

	// 扫盘列表：两个会话都在，标题与带 dir 的回链都在
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions?dir="+enc, nil))
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("列表应 200，得到 %d", rec.Code)
	}
	for _, want := range []string{
		"/sessions/" + testSID + "?dir=" + enc,
		"/sessions/" + oldSID + "?dir=" + enc,
		"先把登录做了",
		"历史会话也能打开",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("列表缺 %q", want)
		}
	}

	// 相对路径 400；存在但从无会话的目录 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions?dir=repo%2Fa", nil))
	if rec.Code != 400 {
		t.Fatalf("相对路径应 400，得到 %d", rec.Code)
	}
	empty := t.TempDir()
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions?dir="+url.QueryEscape(empty), nil))
	if rec.Code != 404 {
		t.Fatalf("无会话目录应 404，得到 %d", rec.Code)
	}

	// 离线历史会话：凭 ?dir= 自证进会话页，返回键回列表
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions/"+oldSID+"?dir="+enc, nil))
	page := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(page, "历史会话也能打开") {
		t.Fatalf("历史会话页应 200 且含首句，code=%d", rec.Code)
	}
	if !strings.Contains(page, "/sessions?dir="+enc) {
		t.Fatalf("从列表进来应给回列表的返回键")
	}

	// 实时流接口同样认 dir 兜底
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET",
		"/api/sessions/"+oldSID+"/stream?from=0&dir="+enc, nil))
	if rec.Code != 200 {
		t.Fatalf("历史会话 stream 应 200，得到 %d", rec.Code)
	}
	var d streamResp
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Events) != 2 {
		t.Fatalf("应读到 2 个事件，得到 %d", len(d.Events))
	}

	// 没有 dir 自证、在线状态里也查无此会话：404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions/"+oldSID+"/stream", nil))
	if rec.Code != 404 {
		t.Fatalf("无 dir 提示的历史会话应 404，得到 %d", rec.Code)
	}
}

// 空态零输入：聚合全部项目最近会话（路径从 transcript cwd 自证）+ JSON 轮询口 + ~ 展开。
func TestSessionRecentAggregateAndAPI(t *testing.T) {
	s, home := setupSession(t, "", nil)
	h := s.Routes()

	// 任意命名的编码目录里放一条带 cwd 的会话
	aggBody := `{"type":"user","uuid":"g1","timestamp":"2026-10-27T12:00:00.000Z","cwd":"/Users/me/proj-agg","message":{"role":"user","content":"聚合页要看到我"}}
{"type":"assistant","uuid":"g2","timestamp":"2026-10-27T12:00:01.000Z","cwd":"/Users/me/proj-agg","message":{"role":"assistant","content":[{"type":"text","text":"在。"}]}}
`
	aggDir := transcript.ProjectDir(home, "/Users/me/proj-agg")
	if err := os.MkdirAll(aggDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aggDir, "sess-agg-0001.jsonl"), []byte(aggBody), 0o600); err != nil {
		t.Fatal(err)
	}
	encAgg := url.QueryEscape("/Users/me/proj-agg")

	// 空态 HTML 直接含聚合行
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions", nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "聚合页要看到我") ||
		!strings.Contains(body, "/sessions/sess-agg-0001?dir="+encAgg) {
		t.Fatalf("空态应聚合最近会话并给带 dir 的链接，code=%d", rec.Code)
	}

	// JSON 轮询口
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions", nil))
	var d struct {
		Rows []struct {
			SID    string `json:"sid"`
			Dir    string `json:"dir"`
			Title  string `json:"title"`
			Online bool   `json:"online"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || len(d.Rows) != 1 {
		t.Fatalf("聚合 JSON 应 1 行，得到 %s", rec.Body.String())
	}
	if d.Rows[0].SID != "sess-agg-0001" || d.Rows[0].Dir != "/Users/me/proj-agg" {
		t.Fatalf("聚合 JSON 行不符: %+v", d.Rows[0])
	}

	// 指定目录的 JSON 口
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions?dir="+encAgg, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "sess-agg-0001") {
		t.Fatalf("按目录 JSON 应 200 含会话，得到 %s", rec.Body.String())
	}

	// ~ 展开：~/不存在 应被当绝对路径处理（404 找不到目录），而非 400 相对路径
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions?dir="+url.QueryEscape("~/no-such-xyz"), nil))
	if rec.Code != 404 {
		t.Fatalf("~/ 应先展开再判存在（期望 404），得到 %d", rec.Code)
	}
}

// 进程表补发现的会话在目录视图里不得串目录：只显示属于该目录的行；
// 从无 transcript 的全新目录里开着的匿名 TUI 也要能显示（不被 404 挡掉）。
func TestSessionListExtrasDirScoping(t *testing.T) {
	projA := t.TempDir()
	projB := t.TempDir()
	const desktopSID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	live := map[string]cliSession{
		"proc-777": {
			SessionID: "proc-777", ID: "proc-777", CWD: projA,
			Status: "waiting", Kind: "interactive", Anon: true,
		},
		desktopSID: {
			SessionID: desktopSID, ID: shortSid(desktopSID), CWD: projB,
			Status: "waiting", Kind: "interactive",
		},
	}
	s, _ := setupSession(t, "", live)
	h := s.Routes()

	// projA 从无 transcript，但有匿名 TUI：200 且只出现 proc-777
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions?dir="+url.QueryEscape(projA), nil))
	if rec.Code != 200 {
		t.Fatalf("有匿名 TUI 的空目录应 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	var d struct {
		Rows []struct {
			SID  string `json:"sid"`
			Anon bool   `json:"anon"`
			Dir  string `json:"dir"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) != 1 || d.Rows[0].SID != "proc-777" || !d.Rows[0].Anon || d.Rows[0].Dir != projA {
		t.Fatalf("projA 应只有匿名行，得到 %+v", d.Rows)
	}

	// projB 视图不得混入 projA 的匿名行
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions?dir="+url.QueryEscape(projB), nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), desktopSID) ||
		strings.Contains(rec.Body.String(), "proc-777") {
		t.Fatalf("projB 应只含桌面会话：%s", rec.Body.String())
	}

	// HTML 视图同样：匿名行可渲染、桌面行不串来
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions?dir="+url.QueryEscape(projA), nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `data-anon="1"`) || strings.Contains(body, desktopSID) {
		t.Fatalf("projA HTML 应只渲染匿名行，code=%d", rec.Code)
	}

	// 聚合视图：两条 extras 都在
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions", nil))
	if rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "proc-777") ||
		!strings.Contains(rec.Body.String(), desktopSID) {
		t.Fatalf("聚合视图应含全部在线会话：%s", rec.Body.String())
	}

	// 匿名空态会话页可打开，stream/history/say 全部拒绝
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions/proc-777?dir="+url.QueryEscape(projA), nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-debug="anon"`) {
		t.Fatalf("匿名会话页应 200 渲染空态，code=%d", rec.Code)
	}
	sayReq := httptest.NewRequest("POST", "/sessions/proc-777/say?dir="+url.QueryEscape(projA),
		strings.NewReader(url.Values{"msg": {"hi"}}.Encode()))
	sayReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, sayReq)
	if rec.Code != 400 {
		t.Fatalf("匿名会话不可代说话，应 400，得到 %d", rec.Code)
	}
}
