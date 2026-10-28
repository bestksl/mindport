package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

// publish 是网页端的放行动作：draft → anchored（默认，锚地待派）；
// 带 --to 时 draft → ready（直接派船）。
var (
	publishTo    string
	publishBerth string
)

var publishCmd = &cobra.Command{
	Use:   "publish <T 编号> [--to 船名] [--berth 项目目录]",
	Short: "放行草稿：默认进锚地待派；--to 直接派船给指定执行者",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.Publish(args[0], defaultActor(), publishTo, publishBerth)
		if err != nil {
			return err
		}
		if publishTo != "" {
			where := ""
			if r.Meta.AssignCwd != "" {
				where = "，船位 " + filepath.Base(r.Meta.AssignCwd)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "🚢 %s 已派船 → %s%s（待接令）: %s\n",
				r.Meta.ID, publishTo, where, r.Meta.Title)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "⚓ %s 已进锚地待派: %s\n", r.Meta.ID, r.Meta.Title)
		}
		return nil
	},
}

func init() {
	publishCmd.Flags().StringVar(&publishTo, "to", "", "直接派船（如 claude@studio / codex@studio），不填则进锚地")
	publishCmd.Flags().StringVar(&publishBerth, "berth", "", "指定船位（项目目录）：只在该目录的会话能接令")
	rootCmd.AddCommand(publishCmd)
}
