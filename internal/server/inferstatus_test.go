package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mkTail(t *testing.T, lines string, age time.Duration) sidFile {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-age)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	return sidFile{path: p, mod: old}
}

const (
	tailBashOpen = `{"type":"assistant","uuid":"a1","timestamp":"2026-11-03T01:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"Bash","input":{"command":"sleep 600"}}]}}` + "\n"
	tailAskOpen  = `{"type":"assistant","uuid":"a2","timestamp":"2026-11-03T01:00:01Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"c2","name":"AskUserQuestion","input":{}}]}}` + "\n"
	tailAskDone  = `{"type":"user","uuid":"u2","timestamp":"2026-11-03T01:00:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"c2","content":"The user answered: x"}]}}` + "\n"
	tailText     = `{"type":"assistant","uuid":"a3","timestamp":"2026-11-03T01:00:03Z","message":{"role":"assistant","content":[{"type":"text","text":"好的"}]}}` + "\n"
)

func TestInferStatus(t *testing.T) {
	cases := []struct {
		name string
		tail string
		age  time.Duration
		want string
	}{
		{"AskUserQuestion 挂起且文件静止 10 分钟=待你输入", tailBashOpen + tailAskOpen, 10 * time.Minute, "waiting"},
		{"Bash 长跑 10 分钟无输出=仍工作中（旧逻辑误报 waiting）", tailBashOpen, 10 * time.Minute, "busy"},
		{"闭合后 assistant 正在落块、2s 内=工作中", tailBashOpen + tailAskOpen + tailAskDone + tailText, 2 * time.Second, "busy"},
		{"全部闭合且静止=等人说话", tailBashOpen + tailAskOpen + tailAskDone + tailText, 10 * time.Minute, "waiting"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferStatus(mkTail(t, tc.tail, tc.age)); got != tc.want {
				t.Fatalf("inferStatus = %q, want %q", got, tc.want)
			}
		})
	}
}
