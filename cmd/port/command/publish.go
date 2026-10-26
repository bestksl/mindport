package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// publish 是调度长官的放行动作：draft → anchored（默认，锚地待派）；
// 带 --to 时 draft → ready（直接派船）。
var publishTo string

var publishCmd = &cobra.Command{
	Use:   "publish <T 编号> [--to 船名]",
	Short: "放行草稿：默认进锚地待派；--to 直接派船给指定执行者",
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
		if publishTo != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "🚢 %s 已派船 → %s（待接令）: %s\n",
				r.Meta.ID, publishTo, r.Meta.Title)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "⚓ %s 已进锚地待派: %s\n", r.Meta.ID, r.Meta.Title)
		}
		return nil
	},
}

func init() {
	publishCmd.Flags().StringVar(&publishTo, "to", "", "直接派船（如 claude@studio / codex@studio），不填则进锚地")
	rootCmd.AddCommand(publishCmd)
}
