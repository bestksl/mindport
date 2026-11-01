package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

// 系统注入（真实落盘形状：后台任务完成时 CLI 以 role=user、content 为
// "<task-notification>…" 字符串写一条记录）不是任何人的发言：
// 时间线上必须整条消失，不能冒充成用户气泡。
func TestTaskNotificationStringDropped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sess.jsonl")
	lines := []string{
		`{"type":"user","uuid":"n1","timestamp":"2026-11-01T09:00:00.000Z","cwd":"/repo","message":{"role":"user","content":"<task-notification>\n<task-id>bxw6r1m5y</task-id>\n<output-file>/tmp/x.output</output-file>\n<tool-use-id>call_x</tool-use-id>\n<status>completed</status>\n<notification-summary>子代理跑完了</notification-summary>\n</task-notification>"}}`,
		`{"type":"user","uuid":"u1","timestamp":"2026-11-01T09:00:01.000Z","cwd":"/repo","message":{"role":"user","content":"继续接着改"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-11-01T09:00:02.000Z","cwd":"/repo","message":{"role":"assistant","content":[{"type":"text","text":"好。"}]}}`,
	}
	if err := os.WriteFile(p, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatal(err)
	}
	pg, err := ReadTail(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(pg.Events) != 2 {
		t.Fatalf("通知记录应整条丢弃，只剩真人+助手 2 条，得到 %d", len(pg.Events))
	}
	if pg.Events[0].Role != "user" || pg.Events[0].Blocks[0].Text != "继续接着改" {
		t.Fatalf("第一条必须是真人发言，得到 %+v", pg.Events[0])
	}
}

// 块数组形态的注入：注入 text 块丢弃；与真人文本混在同一条记录里时，
// 真人文本保留；全是注入则整条记录不落事件。
func TestInjectionBlocksAndMixed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sess.jsonl")
	lines := []string{
		// 纯注入（块形态，含 system-reminder 标签）
		`{"type":"user","uuid":"n2","timestamp":"2026-11-01T10:00:00.000Z","cwd":"/repo","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>别忘了看文件</system-reminder>"}]}}`,
		// 斜杠命令包装也算注入
		`{"type":"user","uuid":"n3","timestamp":"2026-11-01T10:00:01.000Z","cwd":"/repo","message":{"role":"user","content":[{"type":"text","text":"<command-name>foo</command-name><command-args>bar</command-args>"}]}}`,
		// 混合：注入块 + 真人块，只保留真人那块
		`{"type":"user","uuid":"u2","timestamp":"2026-11-01T10:00:02.000Z","cwd":"/repo","message":{"role":"user","content":[{"type":"text","text":"<task-notification>\n<task-id>z</task-id>\n</task-notification>"},{"type":"text","text":"这是我真说的"}]}}`,
		// 对照：正文里引用 XML 的真人消息不能误杀
		`{"type":"user","uuid":"u3","timestamp":"2026-11-01T10:00:03.000Z","cwd":"/repo","message":{"role":"user","content":"你看这个 <foo>bar</foo> 标签啥意思"}}`,
	}
	if err := os.WriteFile(p, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatal(err)
	}
	pg, err := ReadTail(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(pg.Events) != 2 {
		t.Fatalf("应只剩 2 条真人消息，得到 %d", len(pg.Events))
	}
	if got := pg.Events[0].Blocks[0].Text; got != "这是我真说的" {
		t.Fatalf("混合记录只留真人文本，得到 %q", got)
	}
	if len(pg.Events[0].Blocks) != 1 {
		t.Fatalf("混合记录的注入块必须被滤掉，得到 %d 块", len(pg.Events[0].Blocks))
	}
	if got := pg.Events[1].Blocks[0].Text; got != "你看这个 <foo>bar</foo> 标签啥意思" {
		t.Fatalf("引用未知 XML 标签的真人消息不能误杀，得到 %q", got)
	}
}

// 重新生成取回上一句：通知记录绝不能被当成「上一句人话」。
func TestLastHumanTextSkipsInjection(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sess.jsonl")
	lines := []string{
		`{"type":"user","uuid":"u1","timestamp":"2026-11-01T11:00:00.000Z","cwd":"/repo","message":{"role":"user","content":"帮我跑下测试"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-11-01T11:00:01.000Z","cwd":"/repo","message":{"role":"assistant","content":[{"type":"text","text":"跑完了。"}]}}`,
		`{"type":"user","uuid":"n1","timestamp":"2026-11-01T11:00:02.000Z","cwd":"/repo","message":{"role":"user","content":"<task-notification>\n<task-id>z</task-id>\n</task-notification>"}}`,
	}
	if err := os.WriteFile(p, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := LastHumanText(p)
	if !ok {
		t.Fatal("应当找得到真人发言")
	}
	if got != "帮我跑下测试" {
		t.Fatalf("必须跳过通知取真人原话，得到 %q", got)
	}
}

// 目录摘要：通知既不计消息数，也不能当会话标题。
func TestListMetaExcludesInjection(t *testing.T) {
	home := t.TempDir()
	const cwd = "/repo/proj"
	body := `{"type":"user","uuid":"n0","timestamp":"2026-11-01T08:00:00.000Z","cwd":"/repo/proj","message":{"role":"user","content":"<task-notification>\n<task-id>a</task-id>\n</task-notification>"}}
{"type":"user","uuid":"u1","timestamp":"2026-11-01T08:00:01.000Z","cwd":"/repo/proj","message":{"role":"user","content":"先帮我看看这个仓"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-11-01T08:00:02.000Z","cwd":"/repo/proj","message":{"role":"assistant","content":[{"type":"text","text":"在看。"}]}}
{"type":"user","uuid":"n1","timestamp":"2026-11-01T08:00:03.000Z","cwd":"/repo/proj","message":{"role":"user","content":[{"type":"text","text":"<bash-input>ls</bash-input>"}]}}
`
	writeSession(t, home, cwd, "sess-inject-0001", body)
	got, err := ListSessions(home, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("应列出 1 个会话，得到 %d", len(got))
	}
	if got[0].Title != "先帮我看看这个仓" {
		t.Fatalf("标题必须跳过注入取真人首句，得到 %q", got[0].Title)
	}
	if got[0].Events != 2 {
		t.Fatalf("注入记录不计消息数，应 2（真人+助手），得到 %d", got[0].Events)
	}
}
