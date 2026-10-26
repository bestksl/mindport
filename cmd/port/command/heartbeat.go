package command

import (
	"encoding/json"
	"fmt"
	"io"
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
	hbBy        string
	hbProcess   string
	hbInterval  time.Duration
	hbLoop      bool
	hbSession   string
	hbDir       string
	hbClientPID int
)

// hookPayload 是 Claude Code 钩子经 stdin 喂入的 JSON（只取需要的字段）。
type hookPayload struct {
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
}

// heartbeat 由会话钩子（SessionStart / UserPromptSubmit）调用：
// 从 stdin 的钩子 JSON 识别「哪艘船的哪个项目会话」，立即点卯，
// 然后确保有一个脱离会话的后台守护循环在跑。
// 循环每 hbInterval 精确探测本会话 CLI 进程：在就只续 seen，
// 进程退出就删该会话心跳并自毁（别的会话不受影响）。
var heartbeatCmd = &cobra.Command{
	Use:    "heartbeat",
	Short:  "报到并启动后台保活循环（钩子用；会话退出后该船位自动下线）",
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

		sid, dir := hbSession, hbDir
		if sid == "" || dir == "" {
			if p := readHookStdin(); p != nil {
				if sid == "" {
					sid = p.SessionID
				}
				if dir == "" {
					dir = p.Cwd
				}
			}
		}
		if dir == "" {
			dir, _ = os.Getwd()
		}
		if sid == "" {
			// 非钩子环境（手动运行/旧客户端）：以父进程 pid 充当会话 id，
			// 至少让不同终端的会话彼此分开。
			sid = "ppid-" + strconv.Itoa(os.Getppid())
		}

		if hbLoop {
			runHeartbeatLoop(v, by, sid, dir, hbClientPID)
			return nil
		}

		pidPath := heartbeatPidPath(v.Root, by, sid)
		loopPid, oldClient := readHeartbeatPidfile(pidPath)
		clientPID := hbClientPID
		if clientPID == 0 {
			clientPID = findAncestorPID(hbProcess)
		}

		if pidAliveValue(loopPid) && oldClient == clientPID && clientPID > 0 {
			// 同一会话中的每次提问：立刻续一次 seen，循环已在跑。
			return v.KeepaliveSession(by, sid)
		}

		// 新会话（或 resume 后 CLI 换了进程）：报到。task/note 沿用旧文件（resume 场景）。
		if _, err := v.PresentSession(store.Presence{
			Actor: by, SessionID: sid, Dir: dir, PID: clientPID,
		}); err != nil {
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
			"--session", sid,
			"--dir", dir,
			"--pid", strconv.Itoa(clientPID),
			"--loop",
		)
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
		if err := c.Start(); err != nil {
			return err
		}
		if err := os.WriteFile(pidPath,
			[]byte(strconv.Itoa(c.Process.Pid)+" "+strconv.Itoa(clientPID)+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🟢 %s 已报到（船位 %s），保活循环每 %s 探测本会话\n",
			by, filepath.Base(dir), hbInterval)
		return c.Process.Release()
	},
}

// readHookStdin 在 stdin 是管道时读取并解析钩子 JSON；TTY/空/坏数据一律返回 nil。
func readHookStdin() *hookPayload {
	st, err := os.Stdin.Stat()
	if err != nil || (st.Mode()&os.ModeCharDevice) != 0 {
		return nil
	}
	type result struct{ p *hookPayload }
	ch := make(chan result, 1)
	go func() {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<16))
		if err != nil || len(b) == 0 {
			ch <- result{}
			return
		}
		var p hookPayload
		if err := json.Unmarshal(b, &p); err != nil {
			ch <- result{}
			return
		}
		ch <- result{&p}
	}()
	select {
	case r := <-ch:
		return r.p
	case <-time.After(500 * time.Millisecond):
		return nil
	}
}

// findAncestorPID 沿父进程链找最近的名为 name 的进程（hook 链路：
// claude → shell → port）；找不到返回 0（守护循环退化为全局 pgrep 探活）。
func findAncestorPID(name string) int {
	pid := os.Getppid()
	name = strings.ToLower(name)
	for i := 0; i < 8 && pid > 1; i++ {
		out, err := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return 0
		}
		fields := strings.Fields(string(out))
		if len(fields) < 2 {
			return 0
		}
		ppid, _ := strconv.Atoi(fields[0])
		comm := strings.ToLower(filepath.Base(fields[len(fields)-1]))
		if comm == name || strings.Contains(comm, name) {
			return pid
		}
		pid = ppid
	}
	return 0
}

// runHeartbeatLoop 是后台模式主循环：本会话 CLI 在就续期，没了就下线退出。
func runHeartbeatLoop(v *store.Vault, by, sid, dir string, clientPID int) {
	pidPath := heartbeatPidPath(v.Root, by, sid)
	self := os.Getpid()

	tick := func(alive bool) bool {
		// pidfile 被更新的守护循环取代（resume 后新会话接管）→ 安静退出。
		if lp, _ := readHeartbeatPidfile(pidPath); lp != self {
			return false
		}
		if !alive {
			_ = v.AwaySession(by, sid)
			if lp, _ := readHeartbeatPidfile(pidPath); lp == self {
				_ = os.Remove(pidPath)
			}
			return false
		}
		if err := v.KeepaliveSession(by, sid); err != nil {
			// 会话文件丢失（被清理/迁移）：补一次完整报到。
			_, _ = v.PresentSession(store.Presence{
				Actor: by, SessionID: sid, Dir: dir, PID: clientPID,
			})
		}
		return true
	}

	alive := func() bool {
		if clientPID > 0 {
			return syscall.Kill(clientPID, 0) == nil
		}
		return cliAlive() // 兜底：机器上还有同名 CLI 即视为在
	}

	if !tick(alive()) {
		return
	}
	t := time.NewTicker(hbInterval)
	defer t.Stop()
	for range t.C {
		if !tick(alive()) {
			return
		}
	}
}

// cliAlive 用 pgrep -x 精确匹配 CLI 主进程名（无进程线索时的兜底）。
func cliAlive() bool {
	c := exec.Command("pgrep", "-x", hbProcess)
	return c.Run() == nil
}

// heartbeatPidPath 每（船，会话）一个 pidfile：agents/<actor>.<sid8>.hb.pid。
func heartbeatPidPath(vaultRoot, by, sid string) string {
	s := slug(sid)
	if len(s) > 12 {
		s = s[:12]
	}
	return filepath.Join(vaultRoot, ".port", "state", "agents",
		slug(by)+"."+s+".hb.pid")
}

func slug(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-' || r == '_':
			return r
		}
		return '_'
	}, s)
}

// readHeartbeatPidfile 读 "<loopPid> <clientPid>"；缺失/损坏返回 0,0。
func readHeartbeatPidfile(path string) (loopPid, clientPid int) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	f := strings.Fields(string(b))
	if len(f) >= 1 {
		loopPid, _ = strconv.Atoi(f[0])
	}
	if len(f) >= 2 {
		clientPid, _ = strconv.Atoi(f[1])
	}
	return loopPid, clientPid
}

func pidAliveValue(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func init() {
	f := heartbeatCmd.Flags()
	f.StringVar(&hbBy, "by", "", "船名（默认 $PORT_ACTOR 或 <user>@<host>）")
	f.StringVar(&hbProcess, "process", "claude", "要探测的 CLI 主进程名（pgrep -x）")
	f.DurationVar(&hbInterval, "interval", 45*time.Second, "保活探测间隔")
	f.BoolVar(&hbLoop, "loop", false, "后台循环模式（内部使用）")
	f.StringVar(&hbSession, "session", "", "会话 id（正常由钩子 stdin JSON 提供）")
	f.StringVar(&hbDir, "dir", "", "会话项目目录（船位；正常由钩子 stdin JSON 提供）")
	f.IntVar(&hbClientPID, "pid", 0, "本会话 CLI 主进程 pid（内部使用）")
	rootCmd.AddCommand(heartbeatCmd)
}
