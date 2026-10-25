package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// publish 是「人发布、agent 领取」模型的出港动作：draft → ready。
var publishTo string

var publishCmd = &cobra.Command{
	Use:   "publish <T 编号> [--to 执行者]",
	Short: "发布草稿（可同时指派给某个执行者）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.Publish(args[0], defaultActor(), publishTo)
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("⚓ %s 已出港（%s）", r.Meta.ID, impZH(r.Meta.Importance))
		if publishTo != "" {
			msg += "，已指派 " + publishTo
		}
		fmt.Fprintln(cmd.OutOrStdout(), msg)
		return nil
	},
}

func init() {
	publishCmd.Flags().StringVar(&publishTo, "to", "", "委派给指定执行者（如 claude@studio / codex@studio）")
	rootCmd.AddCommand(publishCmd)
}
