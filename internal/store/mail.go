// 调度长信箱（ADR-0008）：会话正在忙、官方拒绝并发 resume 时，网页上的追加指令
// 不丢弃，落进 .port/state/mail/<sid>.jsonl；该会话下一轮提交（UserPromptSubmit）
// 时由 `port mail-pull` 钩子幂等拉走、以 additionalContext 注入，并回标 delivered。
package store

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Mail 是给某个会话的一条留言。Delivered 由拉取钩子回标（重放也不会二次注入）。
type Mail struct {
	ID        string    `json:"id"`
	TS        time.Time `json:"ts"`
	Text      string    `json:"text"`
	Delivered bool      `json:"delivered"`
}

func (v *Vault) mailDir() string {
	return filepath.Join(v.Root, ".port", "state", "mail")
}

func mailPath(dir, sid string) (string, error) {
	if !validSid(sid) {
		return "", fmt.Errorf("会话 id 不合法: %q", sid)
	}
	return filepath.Join(dir, sid+".jsonl"), nil
}

// validSid 只允许 UUID 形态，挡掉路径穿越（sid 全部来自 URL，服务端必须自证）。
func validSid(sid string) bool {
	if sid == "" || len(sid) > 64 {
		return false
	}
	for _, r := range sid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

func newMailID() string {
	var b [8]byte
	_, _ = io.ReadFull(rand.Reader, b[:])
	return "m-" + hex.EncodeToString(b[:])
}

// AppendMail 追加一条未投递留言（0600，信箱内容等同调度指令，不外泄）。
func (v *Vault) AppendMail(sid, text string) (*Mail, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("留言内容不能为空")
	}
	p, err := mailPath(v.mailDir(), sid)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	m := Mail{ID: newMailID(), TS: time.Now(), Text: text}
	line, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	return &m, nil
}

func readMailFile(p string) ([]Mail, error) {
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Mail
	br := bufio.NewReader(f)
	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if trimmed != "" {
				var m Mail
				if json.Unmarshal([]byte(trimmed), &m) == nil && m.ID != "" {
					out = append(out, m)
				}
			}
		}
		if rerr != nil {
			break
		}
	}
	return out, nil
}

// PullMail 原子地取走某会话全部未投递留言并回标 delivered。
// 同一把 vault 锁保证钩子并发/重放时每条留言最多注入一次。
func (v *Vault) PullMail(sid string) ([]Mail, error) {
	p, err := mailPath(v.mailDir(), sid)
	if err != nil {
		return nil, err
	}
	var pending []Mail
	err = v.withLock(func() error {
		all, rerr := readMailFile(p)
		if rerr != nil {
			return rerr
		}
		changed := false
		for i := range all {
			if !all[i].Delivered {
				all[i].Delivered = true
				pending = append(pending, all[i])
				changed = true
			}
		}
		if !changed {
			return nil
		}
		tmp := p + ".tmp"
		f, werr := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if werr != nil {
			return werr
		}
		enc := json.NewEncoder(f)
		for _, m := range all {
			if werr = enc.Encode(m); werr != nil {
				f.Close()
				return werr
			}
		}
		if werr = f.Close(); werr != nil {
			return werr
		}
		return os.Rename(tmp, p)
	})
	if err != nil {
		return nil, err
	}
	return pending, nil
}

// DropMail 删掉某会话的全部留言（拆船时清舱；文件不存在不算错）。
func (v *Vault) DropMail(sid string) error {
	p, err := mailPath(v.mailDir(), sid)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// MailPending 数某会话尚未被钩子拉走的留言（网页提示「N 条排队中」）。
func (v *Vault) MailPending(sid string) (int, error) {
	p, err := mailPath(v.mailDir(), sid)
	if err != nil {
		return 0, err
	}
	all, err := readMailFile(p)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range all {
		if !m.Delivered {
			n++
		}
	}
	return n, nil
}
