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
	case store.StatusReady:
		return "待领取"
	case store.StatusInProgress:
		return "进行中"
	case store.StatusBlocked:
		return "受阻  "
	case store.StatusDone:
		return "已归港"
	case store.StatusCancelled:
		return "已取消"
	default:
		return s
	}
}

func assignee(m store.Meta) string {
	if m.Assignee == nil || strings.TrimSpace(*m.Assignee) == "" {
		return "-"
	}
	return *m.Assignee
}
