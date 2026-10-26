package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Presence 是一个执行者（人 / Claude Code / Codex）的在线心跳。
// 它是运行时状态而非任务数据，存放在 .port/state/agents/，与 vault 内容分离。
type Presence struct {
	Actor  string    `json:"actor"`
	Task   string    `json:"task,omitempty"` // 正在做的任务编号（claim 时自动带上）
	Note   string    `json:"note,omitempty"` // 自由状态行，如「正在跑 go test」
	Seen   time.Time `json:"seen"`
	Online bool      `json:"online"` // ListPresence 按 TTL 算出，不持久化
}

// PresenceTTL：心跳在此时长内视为在线。超时不删文件，灰显「x 分钟前在」。
const PresenceTTL = 2 * time.Minute

// presenceDebounce：20s 内内容未变的心跳不重写文件，
// 让 PostToolUse 这种高频钩子的绝大多数调用变成空操作。
const presenceDebounce = 20 * time.Second

func (v *Vault) agentsDir() string {
	return filepath.Join(v.Root, ".port", "state", "agents")
}

// agentFile 把 actor 名映射成安全文件名（claude@studio → claude_studio.json）。
func agentFile(actor string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-' || r == '_',
			r >= 0x4e00 && r <= 0x9fff: // CJK 保留
			return r
		}
		return '_'
	}, actor)
	return mapped + ".json"
}

// Present 报到 / 刷新心跳。返回是否实际写盘（防抖命中时为 false）。
func (v *Vault) Present(actor, task, note string) (bool, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return false, errors.New("actor 不能为空")
	}
	task = strings.TrimSpace(task)
	note = strings.TrimSpace(note)

	wrote := false
	err := v.withLock(func() error {
		dir := v.agentsDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, agentFile(actor))

		if old, err := readPresenceFile(path); err == nil &&
			old.Task == task && old.Note == note &&
			time.Since(old.Seen) < presenceDebounce {
			return nil
		}

		p := Presence{Actor: actor, Task: task, Note: note, Seen: time.Now()}
		b, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp(dir, ".tmp-*")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		defer os.Remove(tmpName)
		if _, err := tmp.Write(b); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmpName, path); err != nil {
			return err
		}
		wrote = true
		return nil
	})
	return wrote, err
}

// PresentKeepalive 保活：保留上次心跳的 task/note，只刷新 seen（绕过防抖）。
// 心跳守护循环用它续期；还没有心跳文件时按空状态报到。
func (v *Vault) PresentKeepalive(actor string) (bool, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return false, errors.New("actor 不能为空")
	}
	wrote := false
	err := v.withLock(func() error {
		dir := v.agentsDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, agentFile(actor))
		last := Presence{}
		if old, err := readPresenceFile(path); err == nil {
			last = old
		}
		p := Presence{Actor: actor, Task: last.Task, Note: last.Note, Seen: time.Now()}
		b, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp(dir, ".tmp-*")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		defer os.Remove(tmpName)
		if _, err := tmp.Write(b); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmpName, path); err != nil {
			return err
		}
		wrote = true
		return nil
	})
	return wrote, err
}

// Away 显式下线（会话结束）；文件不存在不算错误。
func (v *Vault) Away(actor string) error {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return errors.New("actor 不能为空")
	}
	return v.withLock(func() error {
		err := os.Remove(filepath.Join(v.agentsDir(), agentFile(actor)))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

// ListPresence 列出所有执行者的心跳：在线在前，再按最近出现时间倒序。
func (v *Vault) ListPresence() ([]Presence, error) {
	entries, err := os.ReadDir(v.agentsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	now := time.Now()
	var out []Presence
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p, err := readPresenceFile(filepath.Join(v.agentsDir(), e.Name()))
		if err != nil {
			continue // 损坏/半截文件跳过，不拖垮看板
		}
		p.Online = now.Sub(p.Seen) <= PresenceTTL
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		return out[i].Seen.After(out[j].Seen)
	})
	return out, nil
}

func readPresenceFile(path string) (Presence, error) {
	var p Presence
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	return p, nil
}
