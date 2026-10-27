package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 造两个会话：newer 有真人首句标题，older 首条 user 是工具回报（取不到标题）。
const listNewer = `{"type":"user","uuid":"u1","timestamp":"2026-10-27T10:00:00.000Z","message":{"role":"user","content":"把目录扫盘做了，简单粗暴点"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-27T10:00:05.000Z","message":{"role":"assistant","content":[{"type":"text","text":"好，先 glob。"}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-27T10:00:09.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"done"}]}}
`

const listOlder = `{"type":"user","uuid":"u0","timestamp":"2026-10-26T08:00:00.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu0","content":"warmup"}]}}
{"type":"assistant","uuid":"a0","timestamp":"2026-10-26T08:00:01.000Z","message":{"role":"assistant","content":[{"type":"text","text":"开跑。"}]}}
{"type":"user","uuid":"u0b","timestamp":"2026-10-26T08:00:02.000Z","isSidechain":true,"message":{"role":"user","content":"侧链声音不算标题"}}
`

func writeSession(t *testing.T, home, cwd, sid, body string) {
	t.Helper()
	dir := ProjectDir(home, cwd)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListSessions(t *testing.T) {
	home := t.TempDir()
	const cwd = "/repo/proj"
	writeSession(t, home, cwd, "sess-newer-0001", listNewer)
	writeSession(t, home, cwd, "sess-older-0002", listOlder)

	got, err := ListSessions(home, cwd)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应列出 2 个会话，得到 %d", len(got))
	}
	// LastTS 倒序：newer 在前
	if got[0].SID != "sess-newer-0001" || got[1].SID != "sess-older-0002" {
		t.Fatalf("应按最后更新倒序，得到 %s, %s", got[0].SID, got[1].SID)
	}
	if got[0].Title != "把目录扫盘做了，简单粗暴点" {
		t.Fatalf("首条真人发言应成为标题，得到 %q", got[0].Title)
	}
	if got[0].Events != 3 {
		t.Fatalf("newer 应计 3 个 user/assistant 事件，得到 %d", got[0].Events)
	}
	if got[0].Dir != cwd {
		t.Fatalf("元数据应回填查询目录，得到 %q", got[0].Dir)
	}
	// older 的首条 user 是 tool_result、再下条是 sidechain：标题必须留空
	if got[1].Title != "" {
		t.Fatalf("无真人首句时标题应空，得到 %q", got[1].Title)
	}
	if got[1].Events != 3 {
		t.Fatalf("older 应计 3 事件（含 sidechain user），得到 %d", got[1].Events)
	}
}

func TestListSessionsRejectsRelativeAndMissing(t *testing.T) {
	home := t.TempDir()
	if _, err := ListSessions(home, "relative/path"); err != ErrNotFound {
		t.Fatalf("相对路径应 ErrNotFound，得到 %v", err)
	}
	if _, err := ListSessions(home, "/no/such/project"); err != ErrNotFound {
		t.Fatalf("无 transcript 目录应 ErrNotFound，得到 %v", err)
	}
}

// 造一个带顶层 cwd 的会话文件，放进任意命名的编码目录。
const listCWD = `{"type":"user","uuid":"c1","timestamp":"%s","cwd":"%s","message":{"role":"user","content":"%s"}}
{"type":"assistant","uuid":"c2","timestamp":"%s","cwd":"%s","message":{"role":"assistant","content":[{"type":"text","text":"收到。"}]}}
`

func writeRawSession(t *testing.T, home, encDir, sid, body string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", encDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRecentSessionsAcrossProjects(t *testing.T) {
	home := t.TempDir()
	a := fmt.Sprintf(listCWD, "2026-10-27T09:00:00.000Z", "/Users/me/proj-a", "A 项目的活",
		"2026-10-27T09:00:05.000Z", "/Users/me/proj-a")
	b := fmt.Sprintf(listCWD, "2026-10-26T09:00:00.000Z", "/Users/me/proj-b", "B 项目更早",
		"2026-10-26T09:00:05.000Z", "/Users/me/proj-b")
	// 老格式无 cwd：聚合必须跳过（无法自证路径）
	old := `{"type":"user","timestamp":"2026-10-28T09:00:00.000Z","message":{"role":"user","content":"没 cwd 的古董"}}
`
	writeRawSession(t, home, "-Users-me-proj-a-xyz", "sess-a-0001", a)
	aPath := filepath.Join(home, ".claude", "projects", "-Users-me-proj-a-xyz", "sess-a-0001.jsonl")
	writeRawSession(t, home, "-Users-me-proj-b-zzz", "sess-b-0002", b)
	bPath := filepath.Join(home, ".claude", "projects", "-Users-me-proj-b-zzz", "sess-b-0002.jsonl")
	writeRawSession(t, home, "-legacy-enc", "sess-old-0003", old)
	// 聚合按文件 mtime 排序：显式拉开 a 新 b 旧
	tA := time.Date(2026, 10, 27, 9, 0, 6, 0, time.UTC)
	tB := time.Date(2026, 10, 26, 9, 0, 6, 0, time.UTC)
	if err := os.Chtimes(aPath, tA, tA); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(bPath, tB, tB); err != nil {
		t.Fatal(err)
	}

	got, err := RecentSessions(home, 80, 40)
	if err != nil {
		t.Fatalf("RecentSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("无 cwd 的古董应跳过，应得 2 条，得到 %d", len(got))
	}
	if got[0].SID != "sess-a-0001" || got[0].Dir != "/Users/me/proj-a" {
		t.Fatalf("应按时间倒序且从 cwd 恢复路径，得到 %+v", got[0])
	}
	if got[0].Title != "A 项目的活" {
		t.Fatalf("标题不符: %q", got[0].Title)
	}
	if got[1].Dir != "/Users/me/proj-b" {
		t.Fatalf("第二条路径不符: %+v", got[1])
	}
}

func TestTitleSkipsSystemPseudoPrompt(t *testing.T) {
	home := t.TempDir()
	body := `{"type":"user","timestamp":"2026-10-23T08:00:00.000Z","cwd":"/p","message":{"role":"user","content":"This session is being continued from a previous conversation."}}
{"type":"user","timestamp":"2026-10-23T08:00:01.000Z","cwd":"/p","message":{"role":"user","content":[{"type":"text","text":"继续把目录页优化完"}]}}
`
	writeRawSession(t, home, "-p-x", "sess-c-0001", body)
	got, err := RecentSessions(home, 80, 40)
	if err != nil || len(got) != 1 {
		t.Fatalf("应得 1 条: %v err=%v", got, err)
	}
	if got[0].Title != "继续把目录页优化完" {
		t.Fatalf("compaction 伪首句应跳过，得到 %q", got[0].Title)
	}
}

func TestRecentSessionsUsesLastCWD(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "-enc-moved")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(`{"type":"user","timestamp":"2026-10-20T08:00:00.000Z","cwd":"/old/dir","message":{"role":"user","content":"开头的话"}}` + "\n")
	// 70KB 填充行（>recentHeadBytes），把新 cwd 推到文件尾窗口
	b.WriteString(`{"type":"file-history-snapshot","timestamp":"2026-10-21T08:00:00.000Z","snapshot":{"blob":"` +
		strings.Repeat("x", 70*1024) + `"}}` + "\n")
	b.WriteString(`{"type":"assistant","timestamp":"2026-10-22T08:00:00.000Z","cwd":"/new/dir","message":{"role":"assistant","content":[{"type":"text","text":"好"}]}}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "sess-move-0001.jsonl"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := RecentSessions(home, 80, 40)
	if err != nil || len(got) != 1 {
		t.Fatalf("应得 1 条: %v err=%v", got, err)
	}
	m := got[0]
	if m.Dir != "/new/dir" {
		t.Fatalf("跨目录 resume 后应以尾部 cwd 为当前目录，得到 %q", m.Dir)
	}
	if m.Title != "开头的话" {
		t.Fatalf("标题应取头部真人首句，得到 %q", m.Title)
	}
	if y := m.LastTS.Year(); y != 2026 || m.LastTS.Day() != 22 {
		t.Fatalf("末事件时间应为 10-22，得到 %v", m.LastTS)
	}
}

func TestMetaSingle(t *testing.T) {
	home := t.TempDir()
	writeSession(t, home, "/repo/proj", "sess-only-00003", listNewer)
	m := Meta(home, "/repo/proj", "sess-only-00003")
	if m.Title != "把目录扫盘做了，简单粗暴点" || m.Events != 3 || m.SID != "sess-only-00003" {
		t.Fatalf("单会话摘要不符: %+v", m)
	}
	// 不存在不报错，零值返回
	if m := Meta(home, "/repo/proj", "ghost-0000000000"); m.Events != 0 || m.Title != "" {
		t.Fatalf("缺失会话应零值，得到 %+v", m)
	}
}
