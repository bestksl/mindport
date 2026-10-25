package store

import (
	"errors"
	"strings"
	"testing"
)

func TestThoughtAndTaskLifecycle(t *testing.T) {
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	th, err := v.AddThought("洗澡时的闪念", "", "cli")
	if err != nil {
		t.Fatal(err)
	}
	if th.Meta.Type != "thought" {
		t.Fatalf("thought type = %q", th.Meta.Type)
	}

	t1, err := v.NewTask("第一个任务")
	if err != nil {
		t.Fatal(err)
	}
	if t1.Meta.ID != "T0001" || t1.Meta.Status != StatusReady {
		t.Fatalf("unexpected: %+v", t1.Meta)
	}
	t2, _ := v.NewTask("第二个任务")
	if t2.Meta.ID != "T0002" {
		t.Fatalf("id = %q, want T0002", t2.Meta.ID)
	}

	// 合法流转
	if _, err := v.Claim("T0001", "claude@studio"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Done("T0001", "claude@studio"); err != nil {
		t.Fatal(err)
	}

	// 重新读盘验证持久化与日志追加
	r, err := parseFile(t1.Path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Meta.Status != StatusDone || r.Meta.Assignee == nil || *r.Meta.Assignee != "claude@studio" {
		t.Fatalf("持久化字段错误: %+v", r.Meta)
	}
	if r.Meta.CompletedAt == nil {
		t.Fatal("completed_at 未写入")
	}
	if !contains(r.Body, "claimed by claude@studio") || !contains(r.Body, "done by claude@studio") {
		t.Fatalf("日志缺失:\n%s", r.Body)
	}
	if strings.HasPrefix(r.Body, "---") {
		t.Fatalf("frontmatter 闭合标记残留进正文:\n%s", r.Body)
	}
}

func TestDoubleClaimAndInvalidDone(t *testing.T) {
	v, _ := Open(t.TempDir())
	if _, err := v.NewTask("x"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Claim("T0001", "a@studio"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Claim("T0001", "b@studio"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("重复领取应拒绝，得到 %v", err)
	}
	if _, err := v.NewTask("y"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Done("T0002", "a@studio"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("未领取直接 done 应拒绝，得到 %v", err)
	}
	if _, err := v.Claim("T9999", "a@studio"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在任务应 ErrNotFound，得到 %v", err)
	}
}

func TestListFiltersAndSlug(t *testing.T) {
	v, _ := Open(t.TempDir())
	_, _ = v.AddThought("闪念 A", "", "cli")
	_, _ = v.NewTask("任务 A")
	_, _ = v.NewTask("任务 B")
	_, _ = v.Claim("T0002", "codex@studio")

	thoughts, err := v.ListThoughts()
	if err != nil || len(thoughts) != 1 {
		t.Fatalf("thoughts = %d, err = %v", len(thoughts), err)
	}
	tasks, _ := v.ListTasks("")
	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(tasks))
	}
	busy, _ := v.ListTasks(StatusInProgress)
	if len(busy) != 1 || busy[0].Meta.ID != "T0002" {
		t.Fatalf("in_progress 过滤错误: %+v", busy)
	}
	if s := slug("评估 int8 量化/耗时!!"); !contains(s, "评估") || contains(s, "/") {
		t.Fatalf("slug 异常: %q", s)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
