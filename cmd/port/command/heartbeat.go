package command

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/store"
)

var hbBy string

// hookPayload 是 Claude Code 钩子经 stdin 喂入的 JSON（只取需要的字段）。
type hookPayload struct {
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
}

// heartbeat 由会话钩子（SessionStart / UserPromptSubmit）调用：
// 从 stdin 的钩子 JSON 识别「哪艘船的哪个项目会话」并点卯（写心跳文件，
// 供派船下拉展示船位与最后活动时间）。会话的在线/死亡判定不在本命令：
// serve 每 30s 经官方发现口 `claude agents --json` 获取活会话全集
// （ADR-0007 A 档），所以这里不再 fork 探活守护、不写 pidfile。
var heartbeatCmd = &cobra.Command{
	Use:    "heartbeat",
	Short:  "会话点卯（钩子用；在线状态由调度台经官方发现口判定）",
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

		var sid, dir string
		if p := readHookStdin(); p != nil {
			sid, dir = p.SessionID, p.Cwd
		}
		if dir == "" {
			dir, _ = os.Getwd()
		}
		if sid == "" {
			// 非钩子环境（手动运行/旧客户端）：以父进程 pid 充当会话 id，
			// 至少让不同终端的会话彼此分开。
			sid = "ppid-" + strconv.Itoa(os.Getppid())
		}

		wrote, err := v.PresentSession(store.Presence{
			Actor: by, SessionID: sid, Dir: dir,
		})
		if err != nil {
			return err
		}
		if wrote {
			fmt.Fprintf(cmd.OutOrStdout(), "🟢 %s 已点卯船位：%s\n", by, filepath.Base(dir))
		}
		return nil
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

func init() {
	heartbeatCmd.Flags().StringVar(&hbBy, "by", "", "船名（默认 $PORT_ACTOR 或 <user>@<host>）")
	rootCmd.AddCommand(heartbeatCmd)
}
