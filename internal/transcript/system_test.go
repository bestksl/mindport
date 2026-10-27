package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

// Ctrl-C 中断与压缩续接是「状态」而不是谁发的言：
// 解析期必须转成 role=system 的独立块，不能渲染成用户气泡。
func TestInterruptAndCompactBecomeSystem(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sess.jsonl")
	lines := []string{
		// 新记录：带 interruptedMessageId 字段
		`{"type":"user","uuid":"i1","timestamp":"2026-10-27T10:00:00.000Z","interruptedMessageId":"a9","cwd":"/repo","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
		`{"type":"user","uuid":"u1","timestamp":"2026-10-27T10:00:01.000Z","cwd":"/repo","message":{"role":"user","content":"继续"}}`,
		// 旧记录：没有字段，只有占位句（字符串 content）
		`{"type":"user","uuid":"i2","timestamp":"2026-10-27T10:00:02.000Z","cwd":"/repo","message":{"role":"user","content":"[Request interrupted by user]"}}`,
		// 压缩续接
		`{"type":"user","uuid":"c1","timestamp":"2026-10-27T10:00:03.000Z","isCompactSummary":true,"cwd":"/repo","message":{"role":"user","content":[{"type":"text","text":"This session is being continued from the previous conversation."}]}}`,
	}
	if err := os.WriteFile(p, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatal(err)
	}
	pg, err := ReadTail(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(pg.Events) != 4 {
		t.Fatalf("应得 4 条事件（2 中断 + 1 真人 + 1 压缩），得到 %d", len(pg.Events))
	}
	if pg.Events[0].Role != "system" || len(pg.Events[0].Blocks) != 1 || pg.Events[0].Blocks[0].Kind != "interrupt" {
		t.Fatalf("i1 应为 system/interrupt，得到 %+v", pg.Events[0])
	}
	if pg.Events[1].Role != "user" || pg.Events[1].Blocks[0].Text != "继续" {
		t.Fatalf("u1 应保持真人发言，得到 %+v", pg.Events[1])
	}
	if pg.Events[2].Role != "system" || pg.Events[2].Blocks[0].Kind != "interrupt" {
		t.Fatalf("旧格式占位句 i2 也应识别为 system/interrupt，得到 %+v", pg.Events[2])
	}
	if pg.Events[3].Role != "system" || pg.Events[3].Blocks[0].Kind != "continue" {
		t.Fatalf("c1 应为 system/continue，得到 %+v", pg.Events[3])
	}
}

// 列表元数据：系统标记既不计消息数，也不能当标题。
func TestListExcludesSystemMarkers(t *testing.T) {
	home := t.TempDir()
	const cwd = "/repo/proj"
	body := `{"type":"user","uuid":"i1","timestamp":"2026-10-27T10:00:00.000Z","interruptedMessageId":"a9","cwd":"/repo/proj","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}
{"type":"user","uuid":"u1","timestamp":"2026-10-27T10:00:01.000Z","cwd":"/repo/proj","message":{"role":"user","content":"接着干"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-27T10:00:02.000Z","cwd":"/repo/proj","message":{"role":"assistant","content":[{"type":"text","text":"好。"}]}}
{"type":"user","uuid":"c1","timestamp":"2026-10-27T10:00:03.000Z","isCompactSummary":true,"cwd":"/repo/proj","message":{"role":"user","content":[{"type":"text","text":"This session is being continued."}]}}
`
	writeSession(t, home, cwd, "sess-marker-0001", body)
	got, err := ListSessions(home, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("应列出 1 个会话，得到 %d", len(got))
	}
	if got[0].Title != "接着干" {
		t.Fatalf("标题必须跳过中断标记取真人首句，得到 %q", got[0].Title)
	}
	if got[0].Events != 2 {
		t.Fatalf("系统标记不计消息数，应 2（真人+助手），得到 %d", got[0].Events)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}
