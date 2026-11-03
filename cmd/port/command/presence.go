package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// present 是 agent 的在线状态入口：网页顶栏据此显示在线灯。
// 用法：port present [T编号] [--note 正在做什么] [--by 身份]
var (
	presentBy   string
	presentNote string
)

var presentCmd = &cobra.Command{
	Use:   "present [T 编号]",
	Short: "上线/刷新在线状态（可选带任务号与正在做什么）",
	Long: "向思港报告「我在」。在线状态 2 分钟内有效，网页顶栏显示在线灯；\n" +
		"带任务号时，对应任务卡上显示正在执行。相同内容 20s 内自动防抖，\n" +
		"适合挂在会话钩子上高频调用。",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := presentBy
		if by == "" {
			by = defaultActor()
		}
		task := ""
		if len(args) == 1 {
			task = args[0]
		}
		if _, err := v.Present(by, task, presentNote); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🟢 %s 已上线%s\n", by, taskSuffix(task, presentNote))
		return nil
	},
}

var awayCmd = &cobra.Command{
	Use:   "away",
	Short: "下线（会话结束时调用）",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := presentBy
		if by == "" {
			by = defaultActor()
		}
		if err := v.Away(by); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "⚪ %s 已下线\n", by)
		return nil
	},
}

func taskSuffix(task, note string) string {
	s := ""
	if task != "" {
		s = "，正在做 " + task
	}
	if note != "" {
		if s == "" {
			s = "：" + note
		} else {
			s += "（" + note + "）"
		}
	}
	return s
}

func init() {
	presentCmd.Flags().StringVar(&presentBy, "by", "", "执行者标识（默认 $PORT_ACTOR 或 <user>@<host>）")
	presentCmd.Flags().StringVar(&presentNote, "note", "", "正在做什么（一行状态）")
	awayCmd.Flags().StringVar(&presentBy, "by", "", "执行者标识（默认 $PORT_ACTOR 或 <user>@<host>）")
	rootCmd.AddCommand(presentCmd)
	rootCmd.AddCommand(awayCmd)
}
