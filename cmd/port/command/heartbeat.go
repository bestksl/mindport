package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/store"
)

var (
	hbBy       string
	hbProcess  string
	hbInterval time.Duration
	hbLoop     bool
)

// heartbeat 由会话钩子（SessionStart / UserPromptSubmit）调用：
// 立即点一次卯，然后确保有一个脱离会话的后台守护循环在跑。
// 循环每 hbInterval 探测一次 CLI 主进程：在就续心跳（保留任务号/状态行），
// 进程退出就 away 并自毁。不需要系统级 cron/launchd，随会话自生自灭。
var heartbeatCmd = &cobra.Command{
	Use:    "heartbeat",
	Short:  "报到并启动后台保活循环（钩子用；CLI 退出后自动下线）",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := hbBy
		if by == "" {
			by = defaultActor()
		}
		pidPath := heartbeatPidPath(v.Root, by)

		if hbLoop {
			runHeartbeatLoop(v, by, pidPath)
			return nil
		}

		if pidAlive(pidPath) {
			// 会话中的每次提问：立刻续一次（保留任务号），循环已在跑。
			_, err := v.PresentKeepalive(by)
			return err
		}

		// 新会话首次报到：清掉上一个会话留下的任务号。
		if _, err := v.Present(by, "", ""); err != nil {
			return err
		}

		// 拉起自己的后台循环（独立进程组，父会话退出也不被带走）。
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		c := exec.Command(exe, "heartbeat",
			"--by", by,
			"--process", hbProcess,
			"--interval", hbInterval.String(),
			"--loop",
		)
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
		if err := c.Start(); err != nil {
			return err
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(c.Process.Pid)+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🟢 %s 已报到，保活循环已启动（每 %s 探测 %s 进程）\n",
			by, hbInterval, hbProcess)
		return c.Process.Release()
	},
}

// runHeartbeatLoop 是后台模式主循环：CLI 主进程在就续期，没了就下线退出。
func runHeartbeatLoop(v *store.Vault, by, pidPath string) {
	self := strconv.Itoa(os.Getpid())
	tick := func(alive bool) bool {
		// 被更新的守护循环取代（新会话已起来）→ 安静退出。
		if b, err := os.ReadFile(pidPath); err == nil && strings.TrimSpace(string(b)) != self {
			return false
		}
		if !alive {
			_ = v.Away(by)
			if b, err := os.ReadFile(pidPath); err == nil && strings.TrimSpace(string(b)) == self {
				_ = os.Remove(pidPath)
			}
			return false
		}
		_, _ = v.PresentKeepalive(by)
		return true
	}
	if !tick(cliAlive()) {
		return
	}
	t := time.NewTicker(hbInterval)
	defer t.Stop()
	for range t.C {
		if !tick(cliAlive()) {
			return
		}
	}
}

// cliAlive 用 pgrep -x 精确匹配 CLI 主进程名（daemon/bg-pty-host 不算）。
func cliAlive() bool {
	c := exec.Command("pgrep", "-x", hbProcess)
	return c.Run() == nil
}

func heartbeatPidPath(vaultRoot, by string) string {
	mapped := strings.Map(func(r rune) rune {
		switch r {
		case '@', '/':
			return '_'
		}
		return r
	}, by)
	return filepath.Join(vaultRoot, ".port", "state", "agents", mapped+".hb.pid")
}

func init() {
	f := heartbeatCmd.Flags()
	f.StringVar(&hbBy, "by", "", "船名（默认 $PORT_ACTOR 或 <user>@<host>）")
	f.StringVar(&hbProcess, "process", "claude", "要探测的 CLI 主进程名（pgrep -x）")
	f.DurationVar(&hbInterval, "interval", 45*time.Second, "保活探测间隔")
	f.BoolVar(&hbLoop, "loop", false, "后台循环模式（内部使用）")
	rootCmd.AddCommand(heartbeatCmd)
}

// pidAlive 读 pidfile 并探测进程是否还在；pidfile 失效时顺手清掉。
func pidAlive(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		_ = os.Remove(path)
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		_ = os.Remove(path)
		return false
	}
	return true
}
