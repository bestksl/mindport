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

// Presence 是一个「船位」的在线心跳：同一艘船（actor）可以同时在多个
// 项目会话里在线（SessionID/Dir 区分），也可以是不区分会话的旧客户端/
// 远程 CLI（SessionID 与 Dir 为空）。
// 它是运行时状态而非任务数据，存放在 .port/state/agents/，与 vault 内容分离。
type Presence struct {
	Actor     string    `json:"actor"`
	SessionID string    `json:"session_id,omitempty"` // Claude Code 会话 id；空=不区分会话
	PID       int       `json:"pid,omitempty"`        // 该会话的 CLI 主进程（守护循环精确探活用）
	Dir       string    `json:"dir,omitempty"`        // 会话项目目录（船位）
	Task      string    `json:"task,omitempty"`       // 正在做的任务编号（claim 时自动带上）
	Note      string    `json:"note,omitempty"`       // 自由状态行，如「正在跑 go test」
	Seen      time.Time `json:"seen"`
	Online    bool      `json:"online"` // ListPresence 按 TTL 算出，不持久化
}

// PresenceTTL：心跳在此时长内视为在线。超时不删文件，灰显「x 分钟前在」。
const PresenceTTL = 2 * time.Minute

// presenceDebounce：20s 内内容未变的心跳不重写文件，
// 让高频钩子的绝大多数调用变成空操作。
const presenceDebounce = 20 * time.Second

func (v *Vault) agentsDir() string {
	return filepath.Join(v.Root, ".port", "state", "agents")
}

// slugName 把 actor / session id 映射成安全文件名片段。
func slugName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-' || r == '_',
			r >= 0x4e00 && r <= 0x9fff: // CJK 保留
			return r
		}
		return '_'
	}, s)
}

// sessionPath 会话心跳路径：agents/<actor>/<session>.json。
func sessionPath(dir, actor, sid string) string {
	return filepath.Join(dir, slugName(actor), slugName(sid)+".json")
}

// writePresenceLocked 原子写心跳文件（调用方已持锁、已 MkdirAll）。
func writePresenceLocked(dir, path string, p Presence) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
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
	return os.Rename(tmpName, path)
}

// Present 报到 / 刷新心跳（不区分会话的扁平文件：手动 present、远程 CLI）。
// 返回是否实际写盘（防抖命中时为 false）。
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
		path := filepath.Join(dir, slugName(actor)+".json")
		if old, err := readPresenceFile(path); err == nil &&
			old.Task == task && old.Note == note &&
			time.Since(old.Seen) < presenceDebounce {
			return nil
		}
		if err := writePresenceLocked(dir, path, Presence{Actor: actor, Task: task, Note: note, Seen: time.Now()}); err != nil {
			return err
		}
		wrote = true
		return nil
	})
	return wrote, err
}

// PresentSession 会话级报到（heartbeat 钩子用）：upsert 该会话的心跳文件。
// Task/Note 沿用文件里的现值（心跳不负责任务号），Dir/PID 以本次为准，
// Seen 刷新（20s 防抖）。
func (v *Vault) PresentSession(p Presence) (bool, error) {
	p.Actor = strings.TrimSpace(p.Actor)
	p.SessionID = strings.TrimSpace(p.SessionID)
	if p.Actor == "" || p.SessionID == "" {
		return false, errors.New("actor 与 session_id 不能为空")
	}
	wrote := false
	err := v.withLock(func() error {
		dir := v.agentsDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := sessionPath(dir, p.Actor, p.SessionID)
		now := time.Now()
		if old, err := readPresenceFile(path); err == nil {
			if old.Dir == p.Dir && old.PID == p.PID &&
				old.Task == p.Task && old.Note == p.Note &&
				now.Sub(old.Seen) < presenceDebounce {
				return nil
			}
			// 续报：沿用任务号/状态行（调用方没带就不清）。
			if p.Task == "" {
				p.Task = old.Task
			}
			if p.Note == "" {
				p.Note = old.Note
			}
		}
		p.Seen = now
		if err := writePresenceLocked(dir, path, p); err != nil {
			return err
		}
		wrote = true
		return nil
	})
	return wrote, err
}

// KeepaliveSession 保活：只刷某个会话的 seen，其余字段原样保留（绕过防抖）。
func (v *Vault) KeepaliveSession(actor, sid string) error {
	actor, sid = strings.TrimSpace(actor), strings.TrimSpace(sid)
	if actor == "" || sid == "" {
		return errors.New("actor 与 session_id 不能为空")
	}
	return v.withLock(func() error {
		dir := v.agentsDir()
		path := sessionPath(dir, actor, sid)
		p, err := readPresenceFile(path)
		if err != nil {
			return err // 会话文件没了（可能被手动清），守护循环应重新报到
		}
		p.Seen = time.Now()
		return writePresenceLocked(dir, path, p)
	})
}

// SetActorSessionTask 把任务号写到该船最近活动的会话心跳上（claim 用）；
// 该船没有任何会话文件时退化为扁平心跳（远程 ssh 场景）。
func (v *Vault) SetActorSessionTask(actor, task string) error {
	actor = strings.TrimSpace(actor)
	task = strings.TrimSpace(task)
	return v.withLock(func() error {
		dir := v.agentsDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path, latest := v.newestSessionPathLocked(actor)
		if path == "" {
			p := Presence{Actor: actor, Task: task, Seen: time.Now()}
			return writePresenceLocked(dir, filepath.Join(dir, slugName(actor)+".json"), p)
		}
		latest.Task = task
		latest.Seen = time.Now()
		return writePresenceLocked(dir, path, latest)
	})
}

// ClearActorTask 清掉该船所有心跳上的任务号与状态行（归港用）。
func (v *Vault) ClearActorTask(actor string) error {
	actor = strings.TrimSpace(actor)
	return v.withLock(func() error {
		dir := v.agentsDir()
		flat := filepath.Join(dir, slugName(actor)+".json")
		if p, err := readPresenceFile(flat); err == nil && (p.Task != "" || p.Note != "") {
			p.Task, p.Note, p.Seen = "", "", time.Now()
			if err := writePresenceLocked(dir, flat, p); err != nil {
				return err
			}
		}
		sdir := filepath.Join(dir, slugName(actor))
		entries, err := os.ReadDir(sdir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			path := filepath.Join(sdir, e.Name())
			p, err := readPresenceFile(path)
			if err != nil || (p.Task == "" && p.Note == "") {
				continue
			}
			p.Task, p.Note, p.Seen = "", "", time.Now()
			if err := writePresenceLocked(dir, path, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// newestSessionPathLocked 返回该船最近 seen 的会话心跳文件（调用方持锁）。
func (v *Vault) newestSessionPathLocked(actor string) (string, Presence) {
	dir := v.agentsDir()
	sdir := filepath.Join(dir, slugName(actor))
	entries, err := os.ReadDir(sdir)
	if err != nil {
		return "", Presence{}
	}
	var bestPath string
	var best Presence
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(sdir, e.Name())
		p, err := readPresenceFile(path)
		if err != nil {
			continue
		}
		if bestPath == "" || p.Seen.After(best.Seen) {
			bestPath, best = path, p
		}
	}
	return bestPath, best
}

// AwaySession 某个会话下线：只删该会话文件。文件不存在不算错误。
func (v *Vault) AwaySession(actor, sid string) error {
	actor, sid = strings.TrimSpace(actor), strings.TrimSpace(sid)
	if actor == "" || sid == "" {
		return errors.New("actor 与 session_id 不能为空")
	}
	return v.withLock(func() error {
		err := os.Remove(sessionPath(v.agentsDir(), actor, sid))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

// Away 显式下线（整船）：会话目录与扁平文件都删。
func (v *Vault) Away(actor string) error {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return errors.New("actor 不能为空")
	}
	return v.withLock(func() error {
		dir := v.agentsDir()
		_ = os.RemoveAll(filepath.Join(dir, slugName(actor)))
		err := os.Remove(filepath.Join(dir, slugName(actor)+".json"))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

// ListPresence 列出所有船位心跳：在线在前，再按最近出现时间倒序。
// 兼容旧版扁平文件 agents/<actor>.json 与新版 agents/<actor>/<sid>.json。
func (v *Vault) ListPresence() ([]Presence, error) {
	dir := v.agentsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	now := time.Now()
	var out []Presence
	readInto := func(path string) {
		p, err := readPresenceFile(path)
		if err != nil {
			return // 损坏/半截文件跳过，不拖垮看板
		}
		p.Online = now.Sub(p.Seen) <= PresenceTTL
		out = append(out, p)
	}
	for _, e := range entries {
		if e.IsDir() {
			sub, err := os.ReadDir(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			for _, f := range sub {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
					readInto(filepath.Join(dir, e.Name(), f.Name()))
				}
			}
			continue
		}
		if strings.HasSuffix(e.Name(), ".json") {
			readInto(filepath.Join(dir, e.Name()))
		}
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
