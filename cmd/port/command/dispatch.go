package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

// dispatch 是调度命令：锚地派船、待接令改派、受阻换船（anchored/ready/blocked → ready）。
var (
	dispatchTo    string
	dispatchBerth string
)

var dispatchCmd = &cobra.Command{
	Use:   "dispatch <T 编号> --to 船名 [--berth 项目目录]",
	Short: "派船/改派：锚地派船、待接令改派、受阻换船（调度长官命令）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.Assign(args[0], defaultActor(), dispatchTo, dispatchBerth)
		if err != nil {
			return err
		}
		where := ""
		if r.Meta.AssignCwd != "" {
			where = "，船位 " + filepath.Base(r.Meta.AssignCwd)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🚢 %s 派船 → %s%s（待接令）: %s\n",
			r.Meta.ID, dispatchTo, where, r.Meta.Title)
		return nil
	},
}

func init() {
	dispatchCmd.Flags().StringVar(&dispatchTo, "to", "", "船名（如 claude@studio / codex@studio），必填")
	dispatchCmd.Flags().StringVar(&dispatchBerth, "berth", "", "指定船位（项目目录）：只在该目录的会话能接令")
	_ = dispatchCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(dispatchCmd)
}
