package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestksl/mindport/internal/store"
)

// 类 app 底部 tab bar：看板 / 任务详情 / 船队列表三个主页面都要带，
// 高亮由前端按路径判定，这里只保证结构渲染。
func TestTabbarOnMainPages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	v, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r, err := v.NewTask("导航验收用草稿", "normal", "", true)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{V: v, broker: newBroker()}
	s2, _ := setupSession(t, testTranscript, nil)

	paths := map[string]*Server{
		"/":                    s,
		"/tasks/" + r.Meta.ID:  s,
		"/sessions":            s2,
		"/sessions/" + testSID: s2,
	}
	for path, srv := range paths {
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("GET %s 应 200，得到 %d", path, rec.Code)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`class="tabbar"`,
			`data-tab="board"`,
			`data-tab="fleet"`,
			`id="tab-fleet-badge"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("GET %s 缺 tabbar 结构 %q", path, want)
			}
		}
	}
}

// 手机端看板分段：五个区（草稿/锚地/待接/出海/归港）都要渲染且带计数。
func TestBoardSegmentsRendered(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	v, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.NewTask("草稿一票", "normal", "", true); err != nil {
		t.Fatal(err)
	}
	s := &Server{V: v, broker: newBroker()}

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	for _, seg := range []string{"drafts", "anchored", "ready", "busy", "done"} {
		want := `data-seg="` + seg + `"`
		if !strings.Contains(body, want) {
			t.Fatalf("看板缺分段 %q", seg)
		}
	}
	if !strings.Contains(body, `data-seg="drafts" data-n="1"`) {
		t.Fatalf("草稿分段计数应为 1，输出片段：%s", segSnippet(body))
	}
	for _, label := range []string{"📝 草稿", "⚓ 锚地", "🚢 待接", "🌊 出海", "🏁 归港"} {
		if !strings.Contains(body, label) {
			t.Fatalf("看板分段缺标签 %q", label)
		}
	}
}

func segSnippet(body string) string {
	i := strings.Index(body, "board-seg")
	if i < 0 {
		return ""
	}
	end := i + 400
	if end > len(body) {
		end = len(body)
	}
	return body[i:end]
}
