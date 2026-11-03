// 网页设备接入：一次性配对码 + 设备级会话。
//
// 取代旧版「一个全局共享密钥、cookie 即密钥本体」的模型（旧 cookie 仍兼容
// 放行）：电脑端 `port pair` 铸一个 10 分钟有效的短码，手机/浏览器扫码后
// 交换成该设备独立的 30 天会话凭证，凭证可在 `port devices` 里单独吊销。
//
// 均为运行时状态，存 .port/state/，与任务数据分离；所有写经 vault 同一把锁。
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// PairingTTL：配对码有效期（一次性，过期/已用即拒）。
	PairingTTL = 10 * time.Minute
	// PairingCodeLen：短码长度；32 字符无歧义字母表，8 位约 40bit 熵。
	PairingCodeLen = 8
	pairingAlpha   = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

	// WebSessionDays：设备会话有效期；剩余不足 SessionRenew 时滑动续期。
	WebSessionDays = 30
	sessionRenew   = 7 * 24 * time.Hour
	// sessionTouch：LastSeen 写盘防抖窗口，高频轮询不造成写放大。
	sessionTouch = 60 * time.Second
)

// 配对失败原因分明，调用方据此给不同提示；错误尝试不消耗码。
var (
	ErrPairingInvalid = errors.New("配对码不对")
	ErrPairingExpired = errors.New("配对码已过期")
	ErrPairingUsed    = errors.New("配对码已被使用")
	ErrSessionGone    = errors.New("设备会话不存在或已过期")
)

// PairingCode 是一次配对邀请。码本体不落盘，只存 SHA-256 hash。
type PairingCode struct {
	Hash      string     `json:"hash"`
	Note      string     `json:"note,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// WebSession 是一台通过配对（或密钥登录）接入的设备的独立凭证。
type WebSession struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UA        string    `json:"ua,omitempty"`
	IP        string    `json:"ip,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (v *Vault) pairingPath() string {
	return filepath.Join(v.Root, ".port", "state", "pairing.json")
}

func (v *Vault) sessionsDir() string {
	return filepath.Join(v.Root, ".port", "state", "web-sessions")
}

// randomHex 返回 n 字节随机数的 hex 编码。
func randomHex(n int) (string, error) {
	var raw = make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// pairingCode 用密码学随机数 + 拒绝采样在无歧义字母表上取码（防模偏差）。
func newPairingCode() (string, error) {
	var b strings.Builder
	b.Grow(PairingCodeLen)
	for b.Len() < PairingCodeLen {
		raw := make([]byte, PairingCodeLen*2)
		if _, err := io.ReadFull(rand.Reader, raw); err != nil {
			return "", err
		}
		for _, x := range raw {
			if int(x) < 256-(256%len(pairingAlpha)) {
				b.WriteByte(pairingAlpha[int(x)%len(pairingAlpha)])
				if b.Len() == PairingCodeLen {
					break
				}
			}
		}
	}
	return b.String(), nil
}

func hashPairingCode(code string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return hex.EncodeToString(sum[:])
}

// loadPairingLocked 读取全部配对码（调用方持锁）；文件不存在返回空切片。
func loadPairingLocked(path string) ([]PairingCode, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var codes []PairingCode
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(b, &codes); err != nil {
		return nil, err
	}
	return codes, nil
}

// savePairingLocked 原子写配对码表（调用方持锁）。
func savePairingLocked(path string, codes []PairingCode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if codes == nil {
		codes = []PairingCode{}
	}
	b, err := json.MarshalIndent(codes, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
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

// IssuePairingCode 铸造一个一次性配对码，返回明文码与过期时刻。
// 顺便清掉表里已过期/已使用的旧码。
func (v *Vault) IssuePairingCode(note string) (string, time.Time, error) {
	code, err := newPairingCode()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	exp := now.Add(PairingTTL)
	err = v.withLock(func() error {
		p := v.pairingPath()
		codes, err := loadPairingLocked(p)
		if err != nil {
			return err
		}
		kept := codes[:0]
		for _, c := range codes {
			if c.UsedAt == nil && c.ExpiresAt.After(now) {
				kept = append(kept, c)
			}
		}
		kept = append(kept, PairingCode{
			Hash:      hashPairingCode(code),
			Note:      strings.TrimSpace(note),
			CreatedAt: now,
			ExpiresAt: exp,
		})
		return savePairingLocked(p, kept)
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return code, exp, nil
}

// ConsumePairingCode 校验并一次性消费配对码。码不存在/过期/已用分别报错，
// 这些情况都不改写已用标记；只有成功才原子标 used。
func (v *Vault) ConsumePairingCode(code string) error {
	want := hashPairingCode(code)
	now := time.Now()
	return v.withLock(func() error {
		p := v.pairingPath()
		codes, err := loadPairingLocked(p)
		if err != nil {
			return err
		}
		idx := -1
		var match PairingCode
		for i, c := range codes {
			if subtle.ConstantTimeCompare([]byte(c.Hash), []byte(want)) == 1 {
				idx, match = i, c
				break
			}
		}
		if idx < 0 {
			return ErrPairingInvalid
		}
		if match.UsedAt != nil {
			return ErrPairingUsed
		}
		if !match.ExpiresAt.After(now) {
			return ErrPairingExpired
		}
		used := now
		codes[idx].UsedAt = &used
		return savePairingLocked(p, codes)
	})
}

// CreateWebSession 为一台新接入的设备建立独立会话，返回会话 id（即凭证本体）。
func (v *Vault) CreateWebSession(name, ua, ip string) (*WebSession, error) {
	id, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s := WebSession{
		ID:        id,
		Name:      strings.TrimSpace(name),
		UA:        ua,
		IP:        ip,
		CreatedAt: now,
		LastSeen:  now,
		ExpiresAt: now.AddDate(0, 0, WebSessionDays),
	}
	if s.Name == "" {
		s.Name = "未知设备"
	}
	err = v.withLock(func() error {
		return writeSessionLocked(v.sessionsDir(), s)
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func writeSessionLocked(dir string, s WebSession) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, s.ID+".json")
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

// LookupWebSession 校验设备会话：不存在/已过期返回 ErrSessionGone（过期文件
// 顺手删除）。有效会话按防抖窗口刷新 LastSeen，临期则滑动续 30 天。
func (v *Vault) LookupWebSession(id string) (*WebSession, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, `/\`) {
		return nil, ErrSessionGone
	}
	var out *WebSession
	err := v.withLock(func() error {
		path := filepath.Join(v.sessionsDir(), id+".json")
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return ErrSessionGone
		}
		if err != nil {
			return err
		}
		var s WebSession
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		now := time.Now()
		if !s.ExpiresAt.After(now) {
			_ = os.Remove(path)
			return ErrSessionGone
		}
		if now.Sub(s.LastSeen) >= sessionTouch {
			s.LastSeen = now
			if time.Until(s.ExpiresAt) < sessionRenew {
				s.ExpiresAt = now.AddDate(0, 0, WebSessionDays)
			}
			if err := writeSessionLocked(v.sessionsDir(), s); err != nil {
				return err
			}
		}
		out = &s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListWebSessions 列出全部设备会话，按最近活跃倒序；过期文件顺手清理。
func (v *Vault) ListWebSessions() ([]WebSession, error) {
	var out []WebSession
	err := v.withLock(func() error {
		dir := v.sessionsDir()
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		now := time.Now()
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var s WebSession
			if json.Unmarshal(b, &s) != nil || s.ID == "" {
				continue
			}
			if !s.ExpiresAt.After(now) {
				_ = os.Remove(filepath.Join(dir, e.Name()))
				continue
			}
			out = append(out, s)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeWebSession 吊销一台设备的会话；不存在返回 ErrSessionGone。
func (v *Vault) RevokeWebSession(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, `/\`) {
		return ErrSessionGone
	}
	return v.withLock(func() error {
		path := filepath.Join(v.sessionsDir(), id+".json")
		if _, err := os.Stat(path); err != nil {
			return ErrSessionGone
		}
		return os.Remove(path)
	})
}

// FindSessionByPrefix 按 id 前缀唯一定位设备（命令行手输短前缀用），
// 0 个或多个匹配都报错并附上候选数。
func (v *Vault) FindSessionByPrefix(prefix string) (*WebSession, error) {
	all, err := v.ListWebSessions()
	if err != nil {
		return nil, err
	}
	var hit *WebSession
	for i := range all {
		if strings.HasPrefix(all[i].ID, prefix) {
			if hit != nil {
				return nil, fmt.Errorf("前缀 %s 匹配到多台设备，请用更长的前缀", prefix)
			}
			s := all[i]
			hit = &s
		}
	}
	if hit == nil {
		return nil, ErrSessionGone
	}
	return hit, nil
}
