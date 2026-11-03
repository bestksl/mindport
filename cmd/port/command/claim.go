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
	Short: "领取任务并开始（ready → in_progress）；只有被指派的执行者能领取",
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
			where = "（工作目录：" + filepath.Base(r.Meta.AssignCwd) + "）"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🚢 %s 由 %s 领取并开始%s: %s\n", r.Meta.ID, by, where, r.Meta.Title)
		return nil
	},
}

func init() {
	claimCmd.Flags().StringVar(&claimBy, "by", "", "执行者标识（默认 $PORT_ACTOR 或 <user>@<host>）")
	claimCmd.Flags().StringVar(&claimCwd, "cwd", "", "工作目录/项目目录（默认当前目录；指派时指定了工作目录就必须一致）")
	rootCmd.AddCommand(claimCmd)
}
