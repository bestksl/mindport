package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// dispatch 是调度命令：锚地派船、待接令改派、受阻换船（anchored/ready/blocked → ready）。
var dispatchTo string

var dispatchCmd = &cobra.Command{
	Use:   "dispatch <T 编号> --to 船名",
	Short: "派船/改派：锚地派船、待接令改派、受阻换船（调度长官命令）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.Assign(args[0], defaultActor(), dispatchTo)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🚢 %s 派船 → %s（待接令）: %s\n",
			r.Meta.ID, dispatchTo, r.Meta.Title)
		return nil
	},
}

func init() {
	dispatchCmd.Flags().StringVar(&dispatchTo, "to", "", "船名（如 claude@studio / codex@studio），必填")
	_ = dispatchCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(dispatchCmd)
}
