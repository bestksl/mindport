package transcript

import (
	"os"
	"path/filepath"
	"testing"
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
