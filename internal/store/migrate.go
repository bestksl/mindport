package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MigrateLegacyInbox 把 v0.4 之前 inbox/ 里的旧速记批量建成草稿任务：
// 每条占一个新 T 编号（按捕获时间顺序），正文进「描述」，原文件移入
// archive/inbox-legacy/ 而非删除。已是草稿任务模型的 vault 调用为空操作。
// 返回迁移条数。
func (v *Vault) MigrateLegacyInbox() (int, error) {
	var moved int
	err := v.withLock(func() error {
		dir := filepath.Join(v.Root, "inbox")
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, e.Name())
			}
		}
		if len(names) == 0 {
			return nil
		}
		// 按文件名（前缀是捕获时间 YYYYMMDD-HHMM）迁移，保留捕获顺序。
		sort.Strings(names)
		archiveDir := filepath.Join(v.Root, "archive", "inbox-legacy")
		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			return err
		}
		for _, name := range names {
			src := filepath.Join(dir, name)
			thought, err := parseFile(src)
			if err != nil {
				return fmt.Errorf("解析旧速记 %s 失败: %w", name, err)
			}
			id, err := v.nextID()
			if err != nil {
				return err
			}
			created := thought.Meta.Created
			if created.IsZero() {
				created = time.Now()
			}
			m := Meta{
				Type: "task", ID: id, Title: thought.Meta.Title,
				Status: StatusDraft, Importance: ImpNormal, Created: created,
			}
			desc := strings.TrimSpace(thought.Body)
			if desc == "" {
				desc = thought.Meta.Title
			}
			body := "## 描述\n\n" + desc + "\n\n## 验收标准\n\n## 日志\n" +
				"- " + time.Now().Format("2006-01-02 15:04 -0700") +
				" — migrated from inbox/" + name + "\n"
			path := filepath.Join(v.Root, "tasks", id+"-"+slug(thought.Meta.Title)+".md")
			if err := v.write(path, m, body); err != nil {
				return err
			}
			if err := os.Rename(src, filepath.Join(archiveDir, name)); err != nil {
				return err
			}
			moved++
		}
		return nil
	})
	return moved, err
}
