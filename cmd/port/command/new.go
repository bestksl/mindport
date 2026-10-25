package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	newImp  string
	newGoal string
)

var newCmd = &cobra.Command{
	Use:   "new <标题>",
	Short: "直接创建 ready 任务（重要程度 critical/high/normal/someday，可挂目标）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.NewTask(args[0], newImp, newGoal)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "⚓ %s [%s] 已停泊待领: %s\n",
			r.Meta.ID, impZH(r.Meta.Importance), r.Meta.Title)
		if r.Meta.Goal != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "   目标: %s\n", r.Meta.Goal)
		}
		return nil
	},
}

func init() {
	newCmd.Flags().StringVar(&newImp, "importance", "", "重要程度：critical(关键)/high(重要)/normal(普通)/someday(也许)，默认 normal")
	newCmd.Flags().StringVar(&newGoal, "goal", "", "这个任务服务的目标（目标导向）")
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
