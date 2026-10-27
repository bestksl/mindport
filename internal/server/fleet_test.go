package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFleetStateMapping(t *testing.T) {
	cases := []struct {
		online bool
		status string
		key    string
		text   string
	}{
		{true, "busy", stateWorking, "工作中"},
		{true, "waiting", stateWaiting, "待你输入"},
		{true, "idle", stateIdle, "已停止"},
		{false, "", stateOffline, "已停止"},
	}
	for _, c := range cases {
		got := fleetStateOf(c.online, c.status)
		if got.Key != c.key || got.Text != c.text {
			t.Fatalf("fleetStateOf(%v,%q) = %s/%s，want %s/%s",
				c.online, c.status, got.Key, got.Text, c.key, c.text)
		}
	}
}

func TestSessionMarkAPI(t *testing.T) {
	s, _ := setupSession(t, testTranscript, nil)
	h := s.Routes()

	form := url.Values{"outcome": {"good"}, "dir": {testCWD}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sessions/"+testSID+"/mark", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("标记应 200，得到 %d body=%s", rec.Code, rec.Body.String())
	}
	var d map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || d["text"] != "✅ 圆满结束" {
		t.Fatalf("标记响应错误: %v %s", err, rec.Body.String())
	}
	if mk, ok := s.V.SessionMark(testSID); !ok || mk.Outcome != "good" || mk.Dir != testCWD {
		t.Fatalf("标记未落盘: %+v ok=%v", mk, ok)
	}

	// 船台首屏：结局徽标覆盖实时状态
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sessions/"+testSID, nil))
	body := rec.Body.String()
	if !strings.Contains(body, "badge mark mark-good") || !strings.Contains(body, "圆满结束") {
		t.Fatalf("首屏应渲染圆满结束徽标")
	}

	// stream 带出 mark，前端轮询能切态
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/sessions/"+testSID+"/stream?from=0", nil))
	var sr streamResp
	if err := json.Unmarshal(rec.Body.Bytes(), &sr); err != nil {
		t.Fatal(err)
	}
	if sr.Mark != "good" {
		t.Fatalf("stream 应带 mark=good，得到 %q", sr.Mark)
	}

	// 非法结局 400、非法 sid 404
	rec = httptest.NewRecorder()
	bad := url.Values{"outcome": {"maybe"}}
	req = httptest.NewRequest("POST", "/sessions/"+testSID+"/mark", strings.NewReader(bad.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("非法结局应 400，得到 %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/sessions/evil.sid/mark", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("非法 sid 应 404，得到 %d", rec.Code)
	}

	// 撤结局
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/sessions/"+testSID+"/unmark", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("撤结局应 200，得到 %d", rec.Code)
	}
	if _, ok := s.V.SessionMark(testSID); ok {
		t.Fatalf("撤结局后应查无标记")
	}
}

// 再开口说话 = 结局翻篇：say 成功后旧标记自动撤掉。
func TestSayClearsOutcomeMark(t *testing.T) {
	live := map[string]cliSession{
		testSID: {SessionID: testSID, CWD: testCWD, Status: "busy", State: "working"},
	}
	s, _ := setupSession(t, testTranscript, live)
	if err := s.V.MarkSession(testSID, testCWD, "good"); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"msg": {"再补一条边界用例"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sessions/"+testSID+"/say?dir="+url.QueryEscape(testCWD),
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("say 应 200，得到 %d", rec.Code)
	}
	if _, ok := s.V.SessionMark(testSID); ok {
		t.Fatalf("say 后结局标记应自动撤销")
	}
}

// 任务挂船队：不卡状态机，挂上/换/取下都只动 Meta。
func TestTaskBindFleet(t *testing.T) {
	s, _ := setupSession(t, testTranscript, nil)
	h := s.Routes()
	rec0, err := s.V.NewTask("挂船队的想法", "high", "", true)
	if err != nil {
		t.Fatal(err)
	}
	id := rec0.Meta.ID

	post := func(action string, form url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/tasks/"+id+"/"+action, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := post("bind", url.Values{"fleet": {testSID + "|" + testCWD}})
	if rec.Code != 303 || rec.Header().Get("Location") != "/tasks/"+id {
		t.Fatalf("挂上应 303 回任务页，得到 %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	r, err := s.V.GetTask(id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Meta.Session != testSID || r.Meta.SessionDir != testCWD {
		t.Fatalf("挂载字段未落: %+v", r.Meta)
	}

	// 任务页应显示已挂船队与「取下」
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tasks/"+id, nil))
	body := rec.Body.String()
	if !strings.Contains(body, "bound-fleet") || !strings.Contains(body, "/unbind") {
		t.Fatalf("任务页应显示已挂船队与取下表单")
	}

	// 非法船队：不能挂上去
	post("bind", url.Values{"fleet": {"evil.sid|/x"}})
	r, _ = s.V.GetTask(id)
	if r.Meta.Session != testSID {
		t.Fatalf("非法 sid 不应改动原挂载，得到 %q", r.Meta.Session)
	}

	// 取下
	rec = post("unbind", url.Values{})
	if rec.Code != 303 {
		t.Fatalf("取下应 303，得到 %d", rec.Code)
	}
	r, _ = s.V.GetTask(id)
	if r.Meta.Session != "" || r.Meta.SessionDir != "" {
		t.Fatalf("取下后字段应空: %+v", r.Meta)
	}
}
