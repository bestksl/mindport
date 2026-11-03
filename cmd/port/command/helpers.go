package command

import (
	"path/filepath"
	"strings"

	"github.com/bestksl/mindport/internal/store"
)

func relPath(v *store.Vault, abs string) string {
	if rel, err := filepath.Rel(v.Root, abs); err == nil {
		return rel
	}
	return abs
}

func statusZH(s string) string {
	switch s {
	case store.StatusDraft:
		return "草稿  "
	case store.StatusAnchored:
		return "待办区"
	case store.StatusReady:
		return "待领取"
	case store.StatusInProgress:
		return "进行中"
	case store.StatusBlocked:
		return "受阻  "
	case store.StatusDone:
		return "已完成"
	case store.StatusCancelled:
		return "已取消"
	default:
		return s
	}
}

// impZH 是重要程度的中文展示（目标导向，不谈紧急）。
func impZH(s string) string {
	switch s {
	case store.ImpCritical:
		return "关键"
	case store.ImpHigh:
		return "重要"
	case store.ImpSomeday:
		return "也许"
	default:
		return "普通"
	}
}

func impMark(s string) string {
	switch s {
	case store.ImpCritical:
		return "◆"
	case store.ImpHigh:
		return "●"
	case store.ImpSomeday:
		return "~"
	default:
		return "○"
	}
}

func assignee(m store.Meta) string {
	if m.Assignee == nil || strings.TrimSpace(*m.Assignee) == "" {
		return "-"
	}
	return *m.Assignee
}
