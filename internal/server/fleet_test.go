package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"path/filepath"
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
		{true, "restarting", "restarting", "连接中"},
		{true, "idle", stateIdle, "空闲"},
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

	// 会话页首屏：结局徽标覆盖实时状态
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

// 任务关联会话：不卡状态机，关联/换/取下都只动 Meta。
func TestTaskBindFleet(t *testing.T) {
	s, _ := setupSession(t, testTranscript, nil)
	h := s.Routes()
	rec0, err := s.V.NewTask("关联会话的想法", "high", "", true)
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

	// 任务页应显示已关联会话与「取下」
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tasks/"+id, nil))
	body := rec.Body.String()
	if !strings.Contains(body, "bound-fleet") || !strings.Contains(body, "/unbind") {
		t.Fatalf("任务页应显示已关联会话与取下表单")
	}

	// 非法会话：不能关联
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

// 新开会话表单校验：相对路径 / 不存在的目录必须在调 CLI 之前拦下，
// 且任务不能被关联到任何会话上。
func TestTaskLaunchValidation(t *testing.T) {
	s, _ := setupSession(t, testTranscript, nil)
	h := s.Routes()
	rec0, err := s.V.NewTask("新开会话校验", "high", "", true)
	if err != nil {
		t.Fatal(err)
	}
	id := rec0.Meta.ID

	post := func(form url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/tasks/"+id+"/launch", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.ServeHTTP(rec, req)
		return rec
	}

	for _, tc := range []struct {
		name string
		dir  string
	}{
		{"相对路径", "Documents/GitHub"},
		{"不存在的绝对路径", filepath.Join(t.TempDir(), "nope")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(url.Values{"dir": {tc.dir}, "msg": {"hi"}})
			// v0.8 起 launch 走 fetch：失败就地返 JSON 错误，不再踢回首页丢表单。
			if rec.Code != 400 {
				t.Fatalf("应返 400 就地报错，得到 %d loc=%q", rec.Code, rec.Header().Get("Location"))
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
				t.Fatalf("应带 error 文案，body=%s", rec.Body.String())
			}
		})
	}

	r, _ := s.V.GetTask(id)
	if r.Meta.Session != "" || r.Meta.SessionDir != "" {
		t.Fatalf("校验失败不应产生挂载: %+v", r.Meta)
	}
}

// launchPrompt 要把任务号、标题、目标、正文一次带上。
func TestLaunchPrompt(t *testing.T) {
	s, _ := setupSession(t, testTranscript, nil)
	rec, err := s.V.NewTask("新开船的想法", "normal", "把这事办成", true)
	if err != nil {
		t.Fatal(err)
	}
	rec.Body = "正文里的关键背景"
	p := launchPrompt(rec)
	for _, want := range []string{rec.Meta.ID, rec.Meta.Title, "服务的目标：把这事办成", "正文里的关键背景", "先看清这个任务"} {
		if !strings.Contains(p, want) {
			t.Errorf("开场白缺 %q：\n%s", want, p)
		}
	}
}
