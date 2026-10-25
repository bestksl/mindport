package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var doneBy string

var doneCmd = &cobra.Command{
	Use:   "done <T 编号>",
	Short: "完成任务并归港（in_progress → done）",
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
		r, err := v.Done(args[0], by)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🏁 %s 归港: %s\n", r.Meta.ID, r.Meta.Title)
		return nil
	},
}

func init() {
	doneCmd.Flags().StringVar(&doneBy, "by", "", "执行者标识（默认 $PORT_ACTOR 或 <user>@<host>）")
	rootCmd.AddCommand(doneCmd)
}
