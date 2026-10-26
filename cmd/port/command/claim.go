package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var claimBy string

var claimCmd = &cobra.Command{
	Use:   "claim <T 编号>",
	Short: "接令出港（ready → in_progress）；只有被派船的执行者能接令",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := claimBy
		if by == "" {
			by = defaultActor()
		}
		r, err := v.Claim(args[0], by)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🚢 %s 由 %s 接令出港: %s\n", r.Meta.ID, by, r.Meta.Title)
		return nil
	},
}

func init() {
	claimCmd.Flags().StringVar(&claimBy, "by", "", "执行者标识（默认 $PORT_ACTOR 或 <user>@<host>）")
	rootCmd.AddCommand(claimCmd)
}
