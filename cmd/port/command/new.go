package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	newImp   string
	newGoal  string
	newDraft bool
)

var newCmd = &cobra.Command{
	Use:   "new <标题>",
	Short: "创建任务（默认进待办区等待指派；--draft 先存草稿）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.NewTask(args[0], newImp, newGoal, newDraft)
		if err != nil {
			return err
		}
		verb := "已进待办区，等待指派"
		if newDraft {
			verb = "草稿已存（发布后可指派）"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "⚓ %s [%s] %s: %s\n",
			r.Meta.ID, impZH(r.Meta.Importance), verb, r.Meta.Title)
		if r.Meta.Goal != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "   目标: %s\n", r.Meta.Goal)
		}
		return nil
	},
}

func init() {
	newCmd.Flags().StringVar(&newImp, "importance", "", "重要程度：critical(关键)/high(重要)/normal(普通)/someday(也许)，默认 normal")
	newCmd.Flags().StringVar(&newGoal, "goal", "", "这个任务服务的目标（目标导向）")
	newCmd.Flags().BoolVar(&newDraft, "draft", false, "先存草稿（编辑好后用 port publish 发布到待办区）")
	rootCmd.AddCommand(newCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "初始化 vault 目录结构",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := openVault()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "思港已就绪: %s\n", v.Root)
			return nil
		},
	})
}
