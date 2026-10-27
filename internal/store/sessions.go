// 船队维度（v0.7）：一支船队 = 一个 Claude Code 会话（session）。
// 船队的实时生死在官方发现口里，思港不另造状态；这里只存两类人给的信息：
//   - 结局标记：跑完了、达到预期 → 圆满结束；没跑完就停下 → 中途停止；
//   - 集装箱挂船队：任务 Meta 上记 session/session_dir，想法跟着某支会话走。
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sessionMarksFile = ".port/state/session-marks.json"

// 结局标记取值。
const (
	MarkGood = "good" // 圆满结束：这一轮达到预期
	MarkStop = "stop" // 中途停止：没跑完就停下了
)

// SessionMark 是人给一支船队的结局标记。状态本身（工作中/待输入/停止）
// 以官方发现口为准，标记只表达「人怎么看这个结局」。
type SessionMark struct {
	Outcome string    `json:"outcome"` // good | stop
	At      time.Time `json:"at"`
	Dir     string    `json:"dir,omitempty"` // 标记时的项目目录，兜底定位 transcript
}

func ValidMarkOutcome(o string) bool { return o == MarkGood || o == MarkStop }

func (v *Vault) marksPath() string { return filepath.Join(v.Root, sessionMarksFile) }

// readSessionMarks 读出全量标记；文件不存在/损坏都按空表处理（状态文件，绝不阻塞主链路）。
func (v *Vault) readSessionMarks() map[string]SessionMark {
	out := map[string]SessionMark{}
	b, err := os.ReadFile(v.marksPath())
	if err != nil {
		return out
	}
	_ = json.Unmarshal(b, &out)
	return out
}

func (v *Vault) writeSessionMarks(m map[string]SessionMark) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := v.marksPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, v.marksPath())
}

// MarkSession 给船队打结局标记（重复打即覆盖，时间刷新）。
func (v *Vault) MarkSession(sid, dir, outcome string) error {
	sid = strings.TrimSpace(sid)
	if sid == "" || len(sid) > 64 {
		return errors.New("会话 id 非法")
	}
	if !ValidMarkOutcome(outcome) {
		return fmt.Errorf("结局标记只能是 %q 或 %q", MarkGood, MarkStop)
	}
	return v.withLock(func() error {
		m := v.readSessionMarks()
		m[sid] = SessionMark{Outcome: outcome, At: time.Now(), Dir: strings.TrimSpace(dir)}
		return v.writeSessionMarks(m)
	})
}

// UnmarkSession 取消结局标记。
func (v *Vault) UnmarkSession(sid string) error {
	return v.withLock(func() error {
		m := v.readSessionMarks()
		if _, ok := m[sid]; !ok {
			return nil
		}
		delete(m, sid)
		return v.writeSessionMarks(m)
	})
}

// SessionMarks 返回全量标记副本（列表批量标注用）。
func (v *Vault) SessionMarks() map[string]SessionMark {
	var out map[string]SessionMark
	_ = v.withLock(func() error { out = v.readSessionMarks(); return nil })
	if out == nil {
		out = map[string]SessionMark{}
	}
	return out
}

// SessionMark 取单支船队的标记。
func (v *Vault) SessionMark(sid string) (SessionMark, bool) {
	m, ok := v.readSessionMarks()[sid]
	return m, ok
}

// TasksBySession 返回挂在指定船队上的全部任务（含已完成，按重要程度/编号排序）。
func (v *Vault) TasksBySession(sid string) ([]Record, error) {
	all, err := v.ListTasks("")
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if r.Meta.Session == sid {
			out = append(out, r)
		}
	}
	return out, nil
}

// shortSID 展示/日志用的短 id。
func shortSID(sid string) string {
	if len(sid) > 8 {
		return sid[:8]
	}
	return sid
}

// BindSession 把任务（集装箱）挂到一支船队（会话）上。不卡任务状态机：
// 想法随时可以挂、随时可以换船队，底层只是两个字段 + 一条日志。
func (v *Vault) BindSession(id, sid, dir, by string) (*Record, error) {
	sid = strings.TrimSpace(sid)
	if sid == "" || len(sid) > 64 {
		return nil, errors.New("会话 id 非法")
	}
	dir = strings.TrimSpace(dir)
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		r.Meta.Session = sid
		r.Meta.SessionDir = dir
		touch(&r.Meta)
		line := "挂到船队 " + shortSID(sid)
		if dir != "" {
			line += "（" + filepath.Base(dir) + "）"
		}
		if by != "" {
			line = by + "：" + line
		}
		r.Body = appendLog(r.Body, time.Now(), line)
		if err := v.rewrite(path, r); err != nil {
			return err
		}
		rec = r
		return nil
	})
	return rec, err
}

// UnbindSession 把任务从船队上取下。
func (v *Vault) UnbindSession(id, by string) (*Record, error) {
	var rec *Record
	err := v.withLock(func() error {
		path, err := v.findTaskPath(id)
		if err != nil {
			return err
		}
		r, err := parseFile(path)
		if err != nil {
			return err
		}
		if r.Meta.Session == "" {
			rec = r
			return nil
		}
		line := "从船队取下"
		if by != "" {
			line = by + "：" + line
		}
		r.Meta.Session = ""
		r.Meta.SessionDir = ""
		touch(&r.Meta)
		r.Body = appendLog(r.Body, time.Now(), line)
		if err := v.rewrite(path, r); err != nil {
			return err
		}
		rec = r
		return nil
	})
	return rec, err
}
