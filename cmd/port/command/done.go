package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	doneBy     string
	doneResult string
	doneForce  bool
	doneReason string
)

var doneCmd = &cobra.Command{
	Use:   "done <T 编号> [--result 交付结果] [--force --reason 理由]",
	Short: "归港交差（in_progress/blocked → done）；有验收项须全勾，否则必须 --result",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := doneBy
		if by == "" {
			by = defaultActor()
		}
		r, err := v.Done(args[0], by, doneResult, doneForce, doneReason)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🏁 %s 归港: %s\n", r.Meta.ID, r.Meta.Title)
		if doneForce {
			fmt.Fprintln(cmd.OutOrStdout(), "   ⚠️ 强制归港已记录理由，调度长官会看到")
		}
		return nil
	},
}

func init() {
	f := doneCmd.Flags()
	f.StringVar(&doneBy, "by", "", "船名（默认 $PORT_ACTOR 或 <user>@<host>）")
	f.StringVar(&doneResult, "result", "", "交付结果说明（没有验收项时必填）")
	f.BoolVar(&doneForce, "force", false, "强制归港（验收未完成/无结果时的逃生门）")
	f.StringVar(&doneReason, "reason", "", "强制归港的理由（--force 时必填，永久留痕）")
	rootCmd.AddCommand(doneCmd)
}
