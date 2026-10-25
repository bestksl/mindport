package command

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	checkBy    string
	checkUnset bool
)

// check 勾选/取消验收标准第 n 项：进度 = 验收清单，不是百分比。
var checkCmd = &cobra.Command{
	Use:   "check <T 编号> <序号>",
	Short: "勾选验收标准第 n 项（--unset 取消）",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 {
			return fmt.Errorf("序号必须是正整数，得到 %q", args[1])
		}
		v, err := openVault()
		if err != nil {
			return err
		}
		by := checkBy
		if by == "" {
			by = defaultActor()
		}
		done, total, _, err := v.CheckItem(args[0], n, !checkUnset, by)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s 验收进度 %d/%d\n", args[0], done, total)
		return nil
	},
}

func init() {
	checkCmd.Flags().StringVar(&checkBy, "by", "", "执行者（默认 $PORT_ACTOR 或 <user>@<host>）")
	checkCmd.Flags().BoolVar(&checkUnset, "unset", false, "取消勾选")
	rootCmd.AddCommand(checkCmd)
}
