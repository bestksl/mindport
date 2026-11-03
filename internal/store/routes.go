// 目标线（v0.8）：既定的、长期反复推进的目标。目标线是「虚的概念」——一个 Markdown
// 文件（短码 + 描述），下面挂一串临时性小任务；任务完成即收起，只在目标线页
// 里回看。目标线本身不做事，干活的是为目标线新开的会话（CC 会话）。
//
// 与任务状态机正交：任务靠 Meta.Route 归属目标线，不改任务的任何流转。
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	RouteActive   = "active"
	RouteArchived = "archived"
)

// MaxRouteCodeRunes 是目标线短码上限：会话名形如「数学-1加2」，短码最多 4 字。
const MaxRouteCodeRunes = 4

var (
	// ErrRouteNotFound 表示按编号找不到目标线。
	ErrRouteNotFound = errors.New("目标线不存在")
	// ErrRouteCode 表示短码为空、超长或含不能进会话名的字符。
	ErrRouteCode = errors.New("目标线短码非法")
)

// CreateRoute 建一条目标线（R 编号独立计数）。code 是 1–4 字短码，同 vault 内
// 不能重码；title 是目标线全名，desc 是目标线描述（Markdown 正文）。
func (v *Vault) CreateRoute(title, code, desc string) (*Record, error) {
	title = strings.TrimSpace(title)
	code = cleanRouteCode(code)
	if title == "" {
		return nil, errors.New("目标线名不能为空")
	}
	if err := ValidateRouteCode(code); err != nil {
		return nil, err
	}
	var rec *Record
	err := v.withLock(func() error {
		routes, err := v.listRoutesLocked()
		if err != nil {
			return err
		}
		for _, r := range routes {
			if r.Meta.Code == code {
				return fmt.Errorf("短码 %q 已被目标线「%s」占用", code, r.Meta.Title)
			}
		}
		id, err := v.nextRouteID()
		if err != nil {
			return err
		}
		now := time.Now()
		m := Meta{
			Type: "route", ID: id, Title: title, Code: code,
			Status: RouteActive, Created: now,
		}
		body := "## 描述\n\n" + strings.TrimSpace(desc) + "\n"
		path := filepath.Join(v.Root, "routes", id+"-"+slug(title)+".md")
		if err := v.write(path, m, body); err != nil {
			return err
		}
		rec = &Record{Meta: m, Body: body, Path: path}
		return nil
	})
	return rec, err
}

// GetRoute 按 R 编号取目标线。
func (v *Vault) GetRoute(id string) (*Record, error) {
	var r *Record
	err := v.withLock(func() error {
		path, err := v.findRoutePathLocked(id)
		if err != nil {
			return err
		}
		rec, err := parseFile(path)
		if err != nil {
			return err
		}
		r = rec
		return nil
	})
	return r, err
}

// ListRoutes 列出全部目标线（active 在前，再按编号升序）。
func (v *Vault) ListRoutes() ([]Record, error) {
	var out []Record
	err := v.withLock(func() error {
		routes, err := v.listRoutesLocked()
		if err != nil {
			return err
		}
		out = routes
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Meta.Status == RouteArchived) != (out[j].Meta.Status == RouteArchived) {
			return out[i].Meta.Status != RouteArchived
		}
		return out[i].Meta.ID < out[j].Meta.ID
	})
	return out, err
}

// UpdateRoute 改目标线名/短码/描述。改短码同样查重（排除自身）。
func (v *Vault) UpdateRoute(id, title, code, desc string) (*Record, error) {
	title = strings.TrimSpace(title)
	code = cleanRouteCode(code)
	if title == "" {
		return nil, errors.New("目标线名不能为空")
	}
	if err := ValidateRouteCode(code); err != nil {
		return nil, err
	}
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findRoutePathLocked(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		routes, err := v.listRoutesLocked()
		if err != nil {
			return err
		}
		for _, other := range routes {
			if other.Meta.ID != id && other.Meta.Code == code {
				return fmt.Errorf("短码 %q 已被目标线「%s」占用", code, other.Meta.Title)
			}
		}
		r.Meta.Title = title
		r.Meta.Code = code
		touch(&r.Meta)
		r.Body = "## 描述\n\n" + strings.TrimSpace(desc) + "\n"
		if err := v.rewrite(path, r); err != nil {
			return err
		}
		rec = r
		return nil
	})
	return rec, err
}

// SetRouteStatus 归档 / 恢复目标线。
func (v *Vault) SetRouteStatus(id, status string) error {
	if status != RouteActive && status != RouteArchived {
		return errors.New("目标线状态非法")
	}
	return v.withLock(func() error {
		path, err := v.findRoutePathLocked(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		if r.Meta.Status == status {
			return nil
		}
		r.Meta.Status = status
		touch(&r.Meta)
		return v.rewrite(path, r)
	})
}

// ListTasksByRoute 返回挂在某条目标线下的全部任务（状态过滤同 ListTasks）。
func (v *Vault) ListTasksByRoute(routeID, status string) ([]Record, error) {
	all, err := v.ListTasks(status)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if r.Meta.Route == routeID {
			out = append(out, r)
		}
	}
	return out, nil
}

// SetTaskRoute 把任务挂到目标线（routeID 空串=摘下）。
func (v *Vault) SetTaskRoute(id, routeID string) error {
	routeID = strings.TrimSpace(routeID)
	return v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		if routeID != "" {
			if _, err := v.findRoutePathLocked(routeID); err != nil {
				return err
			}
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		r.Meta.Route = routeID
		touch(&r.Meta)
		return v.rewrite(path, r)
	})
}

func (v *Vault) listRoutesLocked() ([]Record, error) {
	return listDir(filepath.Join(v.Root, "routes"))
}

func (v *Vault) findRoutePathLocked(id string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(v.Root, "routes"))
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), id+"-") && strings.HasSuffix(e.Name(), ".md") {
			return filepath.Join(v.Root, "routes", e.Name()), nil
		}
	}
	return "", fmt.Errorf("%w: 没有 %s", ErrRouteNotFound, id)
}

func (v *Vault) nextRouteID() (string, error) {
	p := filepath.Join(v.Root, ".port", "counter-routes")
	n := 0
	if b, err := os.ReadFile(p); err == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	n++
	if err := os.WriteFile(p, []byte(strconv.Itoa(n)), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("R%04d", n), nil
}

// ValidateRouteCode 校验短码：1–4 字，只收中英文/数字/下划线/中划线，
// 不收空白和会破坏「短码-需求」会话名的标点。
func ValidateRouteCode(code string) error {
	n := utf8.RuneCountInString(code)
	if n < 1 || n > MaxRouteCodeRunes {
		return fmt.Errorf("%w：要 1–%d 个字", ErrRouteCode, MaxRouteCodeRunes)
	}
	for _, r := range code {
		switch {
		case r >= '一' && r <= '鿿':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '—':
		default:
			return fmt.Errorf("%w：%q 只能用中英文、数字、下划线或中划线", ErrRouteCode, string(r))
		}
	}
	return nil
}

// cleanRouteCode 收掉短码两端空白与空白类字符。
func cleanRouteCode(code string) string {
	return strings.TrimSpace(code)
}
