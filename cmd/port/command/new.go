package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <任务标题>",
	Short: "直接创建一个 ready 任务并分配 T 编号（add + promote）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.NewTask(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "⚓ %s 已停泊待领: %s\n", r.Meta.ID, r.Meta.Title)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(newCmd)

	// init：显式初始化 vault（其他命令也会隐式初始化）。
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
