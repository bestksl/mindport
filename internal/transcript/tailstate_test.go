package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeJSONL(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	b := ""
	for _, l := range lines {
		b += l + "\n"
	}
	if err := os.WriteFile(p, []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func asToolUse(id, name string) string {
	return `{"type":"assistant","uuid":"` + id + `-u","timestamp":"2026-11-03T01:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id + `","name":"` + name + `","input":{}}]}}`
}

func asToolResult(id, content string) string {
	q, _ := json.Marshal(content) // 真实 result content 是 JSON 字符串
	return `{"type":"user","uuid":"` + id + `-r","timestamp":"2026-11-03T01:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + id + `","content":` + string(q) + `}]}}`
}

func asAssistantText() string {
	return `{"type":"assistant","uuid":"a-t","timestamp":"2026-11-03T01:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"搞定收工"}]}}`
}

func TestPendingHumanTool(t *testing.T) {
	cases := []struct {
		name     string
		lines    []string
		human    bool
		tool     string
		open     bool
		openTool string
	}{
		{
			name:  "AskUserQuestion 未闭合=等人",
			lines: []string{asToolUse("c1", "Bash"), asToolResult("c1", "ok"), asToolUse("c2", "AskUserQuestion")},
			human: true, tool: "AskUserQuestion",
			open: true, openTool: "AskUserQuestion",
		},
		{
			name:  "AskUserQuestion 已闭合=在跑",
			lines: []string{asToolUse("c1", "AskUserQuestion"), asToolResult("c1", `{"answers":[]}`)},
			human: false,
			open:  false,
		},
		{
			name:  "ExitPlanMode 未闭合=等人",
			lines: []string{asToolUse("c1", "ExitPlanMode")},
			human: true, tool: "ExitPlanMode",
			open: true, openTool: "ExitPlanMode",
		},
		{
			name:  "Bash 未闭合=忙但不是人等",
			lines: []string{asToolUse("c1", "Bash")},
			human: false,
			open:  true, openTool: "Bash",
		},
		{
			name:  "末条纯文本=闭合停着",
			lines: []string{asToolUse("c1", "Bash"), asToolResult("c1", "ok"), asAssistantText()},
			human: false,
			open:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeJSONL(t, tc.lines...)
			name, human := PendingHumanTool(p)
			if human != tc.human || (human && name != tc.tool) {
				t.Fatalf("PendingHumanTool = (%q,%v), want (%q,%v)", name, human, tc.tool, tc.human)
			}
			on, open := OpenToolUse(p)
			if open != tc.open || (open && on != tc.openTool) {
				t.Fatalf("OpenToolUse = (%q,%v), want (%q,%v)", on, open, tc.openTool, tc.open)
			}
		})
	}
}
