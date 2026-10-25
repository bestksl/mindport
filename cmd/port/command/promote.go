package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var promoteImp string

var promoteCmd = &cobra.Command{
	Use:   "promote <inbox 文件名>",
	Short: "把 inbox 闪念转成正式任务（分配 T 编号）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.PromoteThought(filepath.Base(args[0]), promoteImp)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "⚓ 闪念已转任务: %s [%s] %s\n",
			r.Meta.ID, impZH(r.Meta.Importance), r.Meta.Title)
		return nil
	},
}

func init() {
	promoteCmd.Flags().StringVar(&promoteImp, "importance", "", "重要程度：critical/high/normal/someday，默认 normal")
	rootCmd.AddCommand(promoteCmd)
}
