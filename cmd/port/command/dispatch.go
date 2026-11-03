package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

// dispatch 是指派命令：待命区指派、待领取改派、受阻重新指派（anchored/ready/blocked → ready）。
var (
	dispatchTo    string
	dispatchBerth string
)

var dispatchCmd = &cobra.Command{
	Use:   "dispatch <T 编号> --to 执行者 [--berth 项目目录]",
	Short: "指派/重新指派：待命区指派、待领取改派、受阻换人",
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
			where = "，工作目录 " + filepath.Base(r.Meta.AssignCwd)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🚢 %s 已指派 → %s%s（待领取）: %s\n",
			r.Meta.ID, dispatchTo, where, r.Meta.Title)
		return nil
	},
}

func init() {
	dispatchCmd.Flags().StringVar(&dispatchTo, "to", "", "执行者（如 claude@studio / codex@studio），必填")
	dispatchCmd.Flags().StringVar(&dispatchBerth, "berth", "", "指定工作目录（项目目录）：只在该目录的会话能领取")
	_ = dispatchCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(dispatchCmd)
}
