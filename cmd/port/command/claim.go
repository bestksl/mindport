package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	claimBy  string
	claimCwd string
)

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
		cwd := claimCwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		r, err := v.Claim(args[0], by, cwd)
		if err != nil {
			return err
		}
		where := ""
		if r.Meta.AssignCwd != "" {
			where = "（船位：" + filepath.Base(r.Meta.AssignCwd) + "）"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🚢 %s 由 %s 接令出港%s: %s\n", r.Meta.ID, by, where, r.Meta.Title)
		return nil
	},
}

func init() {
	claimCmd.Flags().StringVar(&claimBy, "by", "", "执行者标识（默认 $PORT_ACTOR 或 <user>@<host>）")
	claimCmd.Flags().StringVar(&claimCwd, "cwd", "", "船位/项目目录（默认当前目录；派船指定了船位时必须一致）")
	rootCmd.AddCommand(claimCmd)
}
