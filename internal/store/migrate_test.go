package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateLegacyInbox(t *testing.T) {
	root := t.TempDir()
	v, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "inbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	legacy := func(name, title, body, source string, created time.Time) {
		t.Helper()
		m := Meta{Type: "thought", Title: title, Created: created, Source: source}
		if err := v.write(filepath.Join(root, "inbox", name), m, body); err != nil {
			t.Fatal(err)
		}
	}
	legacy("20261020-1000-first.md", "第一条闪念", "第一条闪念", "cli", now.Add(-2*time.Hour))
	legacy("20261020-1100-second.md", "第二条闪念", "完整描述正文", "mobile", now.Add(-time.Hour))

	n, err := v.MigrateLegacyInbox()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("应迁移 2 条，得到 %d", n)
	}

	all, err := v.ListTasks("")
	if err != nil {
		t.Fatal(err)
	}
	var drafts []Record
	for _, r := range all {
		if r.Meta.Status == StatusDraft {
			drafts = append(drafts, r)
		}
	}
	if len(drafts) != 2 {
		t.Fatalf("应有 2 条草稿，得到 %d", len(drafts))
	}
	byID := map[string]Record{}
	for _, r := range drafts {
		byID[r.Meta.ID] = r
	}
	first, ok1 := byID["T0001"]
	second, ok2 := byID["T0002"]
	if !ok1 || first.Meta.Title != "第一条闪念" {
		t.Fatalf("T0001 字段错误: %+v", first.Meta)
	}
	if !ok2 || second.Meta.Title != "第二条闪念" {
		t.Fatalf("T0002 字段错误: %+v", second.Meta)
	}

	// 原文件移走、inbox 清空、可重复执行
	entries, _ := os.ReadDir(filepath.Join(root, "inbox"))
	if len(entries) != 0 {
		t.Fatalf("迁移后 inbox 应为空，剩 %d 项", len(entries))
	}
	if _, err := os.Stat(filepath.Join(root, "archive", "inbox-legacy", "20261020-1000-first.md")); err != nil {
		t.Fatalf("原件应归档: %v", err)
	}
	if n2, err := v.MigrateLegacyInbox(); err != nil || n2 != 0 {
		t.Fatalf("重复迁移应为空操作，得到 n=%d err=%v", n2, err)
	}
}
