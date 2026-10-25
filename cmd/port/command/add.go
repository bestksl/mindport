package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var addSource string

var addCmd = &cobra.Command{
	Use:   "add <闪念内容>",
	Short: "往 inbox 丢一条闪念（不分配任务编号）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		if addSource == "" {
			addSource = "cli"
		}
		r, err := v.AddThought(args[0], "", addSource)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🌊 闪念已入港: %s\n", relPath(v, r.Path))
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addSource, "source", "cli", "来源标记（cli/mobile/web）")
	rootCmd.AddCommand(addCmd)
}
