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

// 简单粗暴扫盘：不依赖心跳/发现口，?dir= 绝对路径直接翻全部会话，
// 早已离港（无心跳）的会话也能进船台、读实时流接口。
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

	// 再造一艘「无心跳」历史船：只有 transcript 文件
	const oldSID = "sess-hist-00002"
	oldBody := `{"type":"user","uuid":"h1","timestamp":"2026-10-20T09:00:00.000Z","message":{"role":"user","content":"历史会话也能打开"}}
{"type":"assistant","uuid":"h2","timestamp":"2026-10-20T09:00:01.000Z","message":{"role":"assistant","content":[{"type":"text","text":"在的。"}]}}
`
	if err := os.WriteFile(filepath.Join(pdir, oldSID+".jsonl"), []byte(oldBody), 0o600); err != nil {
		t.Fatal(err)
	}
	enc := url.QueryEscape(proj)

	// 无参数：空态页带目录输入框
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `name="dir"`) {
		t.Fatalf("空态应 200 且带目录输入框，code=%d", rec.Code)
	}

	// 扫盘列表：两艘船都在，标题与带 dir 的回链都在
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

	// 无心跳历史船：凭 ?dir= 自证进船台，返回键回列表
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions/"+oldSID+"?dir="+enc, nil))
	page := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(page, "历史会话也能打开") {
		t.Fatalf("历史会话船台应 200 且含首句，code=%d", rec.Code)
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

	// 没有 dir 自证、心跳里也查无此船：404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions/"+oldSID+"/stream", nil))
	if rec.Code != 404 {
		t.Fatalf("无 dir 提示的历史船应 404，得到 %d", rec.Code)
	}
}
