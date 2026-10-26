package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/bestksl/mindport/internal/store"
)

// pollInterval：官方发现口的轮询节奏。芯片页本身 20s 轮询 /api/agents，
// 30s 一轮既跟得上会话出生/死亡，又几乎不占资源。
const pollInterval = 30 * time.Second

// pollTimeout：单次 agents 发现的硬超时。
const pollTimeout = 12 * time.Second

// cliSession 是 `claude agents --json` 的一条会话，只取思港用得到的字段。
// 该命令被官方文档明确列为给 status bar / scheduler 等外部程序读取会话状态的
// 受支持方式（agent view 整体仍是 research preview，故调用一律容错、可整体缺席）。
type cliSession struct {
	PID        int    `json:"pid"`
	ID         string `json:"id"`        // 短 id
	CWD        string `json:"cwd"`       // 会话项目目录（船位）
	Kind       string `json:"kind"`      // interactive | background
	StartedAt  int64  `json:"startedAt"` // 毫秒 epoch
	SessionID  string `json:"sessionId"` // UUID，与心跳文件名一致
	Name       string `json:"name"`      // 会话简介（TUI 的 ai-title）
	Status     string `json:"status"`    // busy | waiting | idle
	State      string `json:"state"`     // working | blocked | done | failed | stopped
	WaitingFor string `json:"waitingFor"`
}

// agentPoller 周期性从官方发现口拉全舰队会话快照：
// 它是会话「在线/死亡」的权威来源，取代旧版每会话 45s pid 探活守护。
type agentPoller struct {
	actor string // 本机 claude 会话统一归属的船名
	exe   string
	v     *store.Vault

	mu   sync.RWMutex
	live map[string]cliSession // sessionId → 最新快照
}

// newAgentPoller 找不到 claude CLI 时返回 nil（调用方回退到 TTL 心跳模型）。
// serve 常由 nohup/launchd 类环境启动，PATH 未必含 ~/.local/bin，补查常见位置。
func newAgentPoller(actor string, v *store.Vault) *agentPoller {
	exe, err := exec.LookPath("claude")
	if err != nil {
		if home, herr := os.UserHomeDir(); herr == nil {
			cand := filepath.Join(home, ".local", "bin", "claude")
			if st, serr := os.Stat(cand); serr == nil && !st.IsDir() {
				exe = cand
			}
		}
	}
	if exe == "" {
		return nil
	}
	return &agentPoller{actor: actor, exe: exe, v: v, live: map[string]cliSession{}}
}

func (p *agentPoller) run(ctx context.Context) {
	p.poll() // 启动即拉一次，免得首屏最多等一个 interval
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.poll()
		}
	}
}

// poll 拉一次快照：成功则整体替换（输出是全集，缺席即会话已死亡），
// 并把会话简介/目录落一份心跳文件（新会话建档，离线后仍在派船下拉里可见）。
// 任何失败都保留上一次快照——CLI 升级中、preview 接口变动都不应让舰队瞬间全灰。
func (p *agentPoller) poll() {
	cctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
	defer cancel()
	b, err := exec.CommandContext(cctx, p.exe, "agents", "--json").Output()
	if err != nil {
		return
	}
	ss, err := parseAgents(b)
	if err != nil {
		return
	}
	live := make(map[string]cliSession, len(ss))
	for _, s := range ss {
		if s.SessionID == "" {
			continue
		}
		live[s.SessionID] = s
		// 建档/刷新：任务号与状态行沿用磁盘现值，简介以官方为准。
		_, _ = p.v.PresentSession(store.Presence{
			Actor: p.actor, SessionID: s.SessionID, Dir: s.CWD,
			PID: s.PID, Name: s.Name, Kind: s.Kind,
		})
	}
	p.mu.Lock()
	p.live = live
	p.mu.Unlock()
}

// parseAgents 解析 `claude agents --json` 输出；坏数据返回错误（调用方保留旧快照）。
func parseAgents(b []byte) ([]cliSession, error) {
	var ss []cliSession
	if err := json.Unmarshal(b, &ss); err != nil {
		return nil, err
	}
	return ss, nil
}

func (p *agentPoller) snapshot() map[string]cliSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]cliSession, len(p.live))
	for k, v := range p.live {
		out[k] = v
	}
	return out
}
