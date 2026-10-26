package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bestksl/mindport/internal/store"
)

func newAuthServer(t *testing.T) (*Server, string) {
	t.Helper()
	v, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{V: v, broker: newBroker()}
	if err := s.EnableAuth(); err != nil {
		t.Fatal(err)
	}
	return s, s.authToken
}

func TestAuthRejectsUnauthenticated(t *testing.T) {
	s, _ := newAuthServer(t)
	h := s.Routes()

	// 网页请求：302 到 /login，并保留想去的路径
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tasks/T1", nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?next=%2Ftasks%2FT1" {
		t.Fatalf("未认证网页应 302 带 next，得到 %d %s", rec.Code, rec.Header().Get("Location"))
	}

	// API 请求：401 JSON，不重定向
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/tasks", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未认证 API 应 401，得到 %d", rec.Code)
	}

	// 公开资产放行
	for _, p := range []string{"/login", "/static/style.css", "/sw.js"} {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusSeeOther {
			t.Fatalf("%s 应公开访问，得到 %d", p, rec.Code)
		}
	}
}

func TestAuthAcceptsCorrectCookie(t *testing.T) {
	s, tok := newAuthServer(t)
	h := s.Routes()

	// 错误密钥：401
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tasks", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: "deadbeef"})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误密钥应 401，得到 %d", rec.Code)
	}

	// 正确密钥：通过鉴权（200，看板可能因空 vault 重定向，但绝不是 401/302 到 login）
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/tasks", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: tok})
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusSeeOther {
		t.Fatalf("正确密钥应放行，得到 %d", rec.Code)
	}
}

func TestLoginFlow(t *testing.T) {
	s, tok := newAuthServer(t)
	h := s.Routes()

	form := url.Values{}
	form.Set("key", tok)
	form.Set("next", "/sessions/abc")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("正确密钥应 303，得到 %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/sessions/abc" {
		t.Fatalf("应回跳到 next，得到 %s", loc)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookieName || cookies[0].Value != tok ||
		!cookies[0].HttpOnly || cookies[0].MaxAge <= 0 {
		t.Fatalf("登录 cookie 属性不对: %+v", cookies)
	}

	// 错误密钥：留在登录页并出错误文案（不设 cookie）
	form2 := url.Values{}
	form2.Set("key", "wrong")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/login", strings.NewReader(form2.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "密钥不对") {
		t.Fatalf("错误密钥应重新渲染登录页带错误，得到 %d", rec.Code)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	s, _ := newAuthServer(t)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/logout", nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("登出应 303 回 /login，得到 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("登出应回写过期 cookie: %+v", rec.Result().Cookies())
	}
}

func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"/tasks/T1":       "/tasks/T1",
		"/sessions/x?a=1": "/sessions/x?a=1",
		"":                "/",
		"//evil.com/x":    "/",
		`/\evil`:          "/", // 浏览器会把 /\ 规范化成 //
		"https://evil":    "/",
	}
	for in, want := range cases {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}
