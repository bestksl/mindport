// serve 运行状态：serve 进程把自己的监听信息写到 .port/state/serve.json，
// 供 `port pair` 之类的同伴命令发现正在运行的服务并铸配对码。
// 不做存活担保——pair 发现失联/端口不符时会提示用户。
package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ServeState 是正在运行的 serve 进程的自描述信息。
type ServeState struct {
	Host      string    `json:"host"`
	Port      string    `json:"port"`
	Auth      bool      `json:"auth"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// ServeStatePath 是运行状态文件路径。
func ServeStatePath(root string) string {
	return filepath.Join(root, ".port", "state", "serve.json")
}

// WriteServeState 原子写运行状态（0644：同机其他 CLI 命令要能读）。
func WriteServeState(root string, st ServeState) error {
	p := ServeStatePath(root)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
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
	return os.Rename(tmpName, p)
}

// ReadServeState 读运行状态；文件不存在返回 (nil, nil)。
func ReadServeState(root string) (*ServeState, error) {
	b, err := os.ReadFile(ServeStatePath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st ServeState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// RemoveServeState 删除运行状态文件（serve 退出时清理；不存在不报错）。
func RemoveServeState(root string) error {
	if err := os.Remove(ServeStatePath(root)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
