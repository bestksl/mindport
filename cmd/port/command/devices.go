package command

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// devicesCmd 管理通过配对/密钥登录过网页的设备：每台设备有独立会话凭证，
// 丢手机或借出设备后可单独吊销，不影响其他设备。
var devicesCmd = &cobra.Command{
	Use:   "devices [list|revoke <id前缀>]",
	Short: "管理登录过网页的设备（查看 / 吊销访问）",
	Long: "管理登录过网页的设备（手机/浏览器）。\n" +
		"每台设备配对后持有独立的 30 天会话凭证，可单独吊销：\n" +
		"  port devices              列出已授权设备\n" +
		"  port devices revoke ab12  按 id 前缀吊销一台设备",
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		action := "list"
		prefix := ""
		if len(args) >= 1 {
			action = args[0]
		}
		if len(args) == 2 {
			prefix = args[1]
		}

		switch action {
		case "list", "ls":
			sessions, err := v.ListWebSessions()
			if err != nil {
				return err
			}
			if len(sessions) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "还没有设备登录过。手机接入：port pair。")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\t设备\t来源 IP\t最近活跃\t签发于")
			for _, s := range sessions {
				short := s.ID
				if len(short) > 10 {
					short = short[:10]
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					short, s.Name, dash(s.IP), relTime(s.LastSeen), s.CreatedAt.Format("01-02 15:04"))
			}
			tw.Flush()
			fmt.Fprintln(cmd.OutOrStdout(), "\n吊销一台：port devices revoke <ID>")
			return nil
		case "revoke", "rm":
			if prefix == "" {
				return fmt.Errorf("用法：port devices revoke <id 前缀>")
			}
			sess, err := v.FindSessionByPrefix(prefix)
			if err != nil {
				return err
			}
			if err := v.RevokeWebSession(sess.ID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "已吊销设备 %s（%s）的访问，它的下一次请求即被登出。\n",
				sess.ID[:10], sess.Name)
			return nil
		default:
			return fmt.Errorf("未知子命令 %q（可用：list / revoke）", action)
		}
	},
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// relTime 给设备列表一个紧凑的「多久前」。
func relTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
}

func init() {
	rootCmd.AddCommand(devicesCmd)
}
