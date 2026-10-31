package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathEncoding(t *testing.T) {
	got := Path("/Users/jmzx", "/Users/jmzx/studio/x.app", "abc-123")
	want := filepath.Join("/Users/jmzx", ".claude", "projects", "-Users-jmzx-studio-x-app", "abc-123.jsonl")
	if got != want {
		t.Fatalf("编码错误:\n got %s\nwant %s", got, want)
	}
}

// 构造一份典型会话：真人发言 / 助手思考+文本+工具 / 工具结果 / 内部噪音 / 坏行。
func writeFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sess.jsonl")
	lines := []string{
		`{"type":"user","uuid":"u1","timestamp":"2026-10-26T10:00:00.000Z","message":{"role":"user","content":"先把登录做了"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-10-26T10:00:01.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"thinking","thinking":"先看结构"},{"type":"text","text":"好，我先加登录页。"},{"type":"tool_use","id":"tu1","name":"Edit","input":{"file_path":"/repo/auth.go"}}]}}`,
		`{"type":"user","uuid":"r1","timestamp":"2026-10-26T10:00:02.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"done"}]}}`,
		`{"type":"api-request","uuid":"x1","message":{}}`,
		`{broken json`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseFixture(t *testing.T) {
	p := writeFixture(t)
	pg, err := ReadTail(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(pg.Events) != 2 { // u1 / a1（r1 已并入 a1 的工具卡）；噪音与坏行被跳过
		t.Fatalf("应解析出 2 条事件（工具回报并入工具卡），得到 %d", len(pg.Events))
	}
	if pg.Events[0].ID != "u1" || pg.Events[0].Blocks[0].Kind != "text" ||
		pg.Events[0].Blocks[0].Text != "先把登录做了" {
		t.Fatalf("真人消息解析错误: %+v", pg.Events[0])
	}
	a := pg.Events[1]
	kinds := map[string]bool{}
	var tool *Block
	for i := range a.Blocks {
		kinds[a.Blocks[i].Kind] = true
		if a.Blocks[i].Kind == "tool" {
			tool = &a.Blocks[i]
		}
	}
	if !kinds["thinking"] || !kinds["text"] || !kinds["tool"] {
		t.Fatalf("助手块类型缺失: %+v", a.Blocks)
	}
	if tool.ToolID != "tu1" || tool.Name != "Edit" || !strings.Contains(tool.Summary, "auth.go") {
		t.Fatalf("工具块摘要错误: %+v", tool)
	}
	// 工具回报并入同一工具块，而不是单列成一条 role=user 伪消息
	if tool.Result != "done" || tool.ResultErr {
		t.Fatalf("工具回报应并入工具卡: %+v", tool)
	}
	if pg.Cursor != pg.Size {
		t.Fatalf("读全量后游标应到文件尾: cursor=%d size=%d", pg.Cursor, pg.Size)
	}
}

func TestIncrementalIdempotent(t *testing.T) {
	p := writeFixture(t)
	first, err := ReadSince(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 2 { // u1 / a1（r1 并入 a1 工具卡）
		t.Fatalf("首次增量应得 2 条，得到 %d", len(first.Events))
	}
	// 同游标重复请求：幂等，无新事件
	again, _ := ReadSince(p, first.Cursor)
	if len(again.Events) != 0 || again.Cursor != first.Cursor {
		t.Fatalf("重复请求应返回空且游标不动，得到 %+v", again)
	}
	// 追加一条新消息后，只出新事件
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"type":"assistant","uuid":"a2","timestamp":"2026-10-26T10:01:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"完成"}]}}` + "\n")
	f.Close()
	got, err := ReadSince(p, first.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || got.Events[0].ID != "a2" {
		t.Fatalf("追加后应只得 a2，得到 %+v", got.Events)
	}
}

func TestPartialLineNotLost(t *testing.T) {
	p := writeFixture(t)
	base, _ := os.Stat(p)
	// 写一条还没换行的半行（正撞写盘）
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"type":"assistant","uuid":"a3","message":{"role":"assistant","content":[{"type":"text","text":"半`)
	f.Close()

	pg, err := ReadSince(p, base.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(pg.Events) != 0 {
		t.Fatalf("半行不应被解析，得到 %d 条", len(pg.Events))
	}
	if pg.Cursor != base.Size() {
		t.Fatalf("游标应停在半行行首，得到 %d（基线 %d）", pg.Cursor, base.Size())
	}

	// 补全换行：下一轮必须读到完整事件，不丢不重
	f, _ = os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`条"}]}}` + "\n")
	f.Close()
	pg, err = ReadSince(p, pg.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(pg.Events) != 1 || pg.Events[0].ID != "a3" ||
		pg.Events[0].Blocks[0].Text != "半条" {
		t.Fatalf("补全后应得完整 a3，得到 %+v", pg.Events)
	}
}

// 错误回报并入工具卡时必须带上失败标记。
func TestFoldToolError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sess.jsonl")
	lines := []string{
		`{"type":"user","uuid":"u1","timestamp":"2026-10-26T10:00:00.000Z","message":{"role":"user","content":"跑一下"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-10-26T10:00:01.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"go test"}}]}}`,
		`{"type":"user","uuid":"r1","timestamp":"2026-10-26T10:00:02.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b1","content":"FAIL","is_error":true}]}}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pg, _ := ReadTail(p)
	if len(pg.Events) != 2 {
		t.Fatalf("错误回报也应并入工具卡（2 条事件），得到 %d", len(pg.Events))
	}
	tb := pg.Events[1].Blocks[0]
	if tb.Kind != "tool" || tb.Result != "FAIL" || !tb.ResultErr {
		t.Fatalf("工具块应带错误回报与标记: %+v", tb)
	}
}

// 找不到归属工具的回报（tool_use 在更早读取窗口）保留为独立 result 块，不丢。
func TestFoldUnmatchedResultKept(t *testing.T) {
	evs := []Event{
		{ID: "r1", Role: "user", TS: "t", Blocks: []Block{{Kind: "result", ToolID: "ghost", Text: "orphan"}}},
	}
	out := foldToolResults(evs)
	if len(out) != 1 || out[0].Blocks[0].Kind != "result" {
		t.Fatalf("无归属回报应保留: %+v", out)
	}
}

// 重新生成要取回最近一条真人原话，工具结果/中断/助手话都不算。
func TestLastHumanText(t *testing.T) {
	p := writeFixture(t)
	txt, ok := LastHumanText(p)
	if !ok || txt != "先把登录做了" {
		t.Fatalf("应取到最近真人原话，得到 %q ok=%v", txt, ok)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(
		`{"type":"assistant","uuid":"a9","timestamp":"2026-10-26T10:02:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"z1","name":"Read","input":{"file_path":"/x"}}]}}` + "\n" +
			`{"type":"user","uuid":"rz","timestamp":"2026-10-26T10:02:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"z1","content":"x"}]}}` + "\n" +
			`{"type":"user","uuid":"u9","timestamp":"2026-10-26T10:02:02.000Z","message":{"role":"user","content":"再改成蓝色"}}` + "\n")
	f.Close()
	txt, ok = LastHumanText(p)
	if !ok || txt != "再改成蓝色" {
		t.Fatalf("应越过工具回报取到最新真人话，得到 %q ok=%v", txt, ok)
	}
}
