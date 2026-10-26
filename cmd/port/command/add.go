package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var addImp string

// addCmd 入港即立箱：一条念头进来直接就是草稿任务（占 T 编号），
// 不再有独立的闪念/inbox 与「转集装箱」步骤。打磨与放行都在任务详情完成。
var addCmd = &cobra.Command{
	Use:   "add <任务内容>",
	Short: "入港立箱：直接建一条草稿任务",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.NewTask(args[0], addImp, "", true)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🧱 已立箱 %s [%s]（草稿·备货中）: %s\n    %s\n",
			r.Meta.ID, impZH(r.Meta.Importance), r.Meta.Title, relPath(v, r.Path))
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addImp, "importance", "", "重要程度：critical/high/normal/someday，默认 normal")
	rootCmd.AddCommand(addCmd)
}
