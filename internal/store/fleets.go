// 船队名册（v0.8）：记网页端开出去的船队（fleet bridge）的人给信息——
// 船名（短码-需求）、项目目录、挂的任务与航线。生死与实时状态仍以 CC 官方
// 发现口/桥进程为准，这里不复制。Archived=true 表示已从主列表折叠（归港），
// Remove 是「拆船」（等效 CC 的 Ctrl-X）：只清思港侧记录，不碰 CC transcript。
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

const fleetsFile = ".port/state/fleets.json"

// FleetInfo 是一支网页船队的名册条目。
type FleetInfo struct {
	SID    string    `json:"sid"`
	Name   string    `json:"name"`           // 船名（如 数学-1加2），即 CC 会话 -n
	Dir    string    `json:"dir"`            // 项目目录（CC 的 cwd）
	Task   string    `json:"task,omitempty"` // 挂的任务 T 编号（可空）
	Route  string    `json:"route,omitempty"`
	Born   time.Time `json:"born"`
	Hidden bool      `json:"hidden,omitempty"` // 已拆船（Ctrl-X）：从思港列表隐藏，CC transcript 不动
}

func (v *Vault) fleetsPath() string { return filepath.Join(v.Root, fleetsFile) }

func (v *Vault) readFleets() (map[string]FleetInfo, error) {
	out := map[string]FleetInfo{}
	b, err := os.ReadFile(v.fleetsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		// 状态文件损坏不阻塞主链路：按空表起步，下次写入自愈。
		return map[string]FleetInfo{}, nil
	}
	return out, nil
}

func (v *Vault) writeFleets(m map[string]FleetInfo) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := v.fleetsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, v.fleetsPath())
}

// SaveFleet 登记/更新一支船队（按 sid upsert）。
func (v *Vault) SaveFleet(info FleetInfo) error {
	info.SID = strings.TrimSpace(info.SID)
	if len(info.SID) > 64 {
		return errors.New("会话 id 非法")
	}
	if info.SID == "" {
		return errors.New("会话 id 不能为空")
	}
	info.Name = strings.TrimSpace(info.Name)
	return v.withLock(func() error {
		m, err := v.readFleets()
		if err != nil {
			return err
		}
		if old, ok := m[info.SID]; ok {
			// 只覆盖非空字段：任务页挂载等后到的更新不抹掉船名/目录。
			if info.Name == "" {
				info.Name = old.Name
			}
			if info.Dir == "" {
				info.Dir = old.Dir
			}
			if info.Task == "" {
				info.Task = old.Task
			}
			if info.Route == "" {
				info.Route = old.Route
			}
			if info.Born.IsZero() {
				info.Born = old.Born
			}
			info.Hidden = old.Hidden // 已拆船的会话不会因补登而复活
		} else if info.Born.IsZero() {
			info.Born = time.Now()
		}
		m[info.SID] = info
		return v.writeFleets(m)
	})
}

// Fleet 取一条名册（含已拆船的隐藏条目，调用方按 Hidden 判断）。
func (v *Vault) Fleet(sid string) (FleetInfo, bool) {
	m, _ := v.readFleets()
	info, ok := m[sid]
	return info, ok
}

// Fleets 返回全部未拆船名册条目（出生时间倒序，新的在前）。
func (v *Vault) Fleets() []FleetInfo {
	m, _ := v.readFleets()
	out := make([]FleetInfo, 0, len(m))
	for _, f := range m {
		if !f.Hidden {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Born.After(out[j].Born) })
	return out
}

// hiddenSIDs 返回已拆船会话集合（列表过滤/船台 404 用）。
func (v *Vault) HiddenSIDs() map[string]bool {
	m, _ := v.readFleets()
	out := map[string]bool{}
	for sid, f := range m {
		if f.Hidden {
			out[sid] = true
		}
	}
	return out
}

// HideFleet 拆船（Ctrl-X）：名册条目标记隐藏，无名册则建一条隐藏占位
// （让 CLI 开的老会话也能从思港抹掉）。CC 进程与 transcript 不在此处理。
func (v *Vault) HideFleet(sid, dir, name string) error {
	sid = strings.TrimSpace(sid)
	if sid == "" || len(sid) > 64 {
		return errors.New("会话 id 非法")
	}
	return v.withLock(func() error {
		m, err := v.readFleets()
		if err != nil {
			return err
		}
		info := m[sid]
		info.SID = sid
		info.Hidden = true
		if dir != "" {
			info.Dir = dir
		}
		if name != "" {
			info.Name = name
		}
		if info.Born.IsZero() {
			info.Born = time.Now()
		}
		m[sid] = info
		return v.writeFleets(m)
	})
}
