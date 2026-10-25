package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var blockBy string

var blockCmd = &cobra.Command{
	Use:   "block <T 编号> <原因>",
	Short: "标记任务受阻（in_progress → blocked，必须给原因）",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := blockBy
		if by == "" {
			by = defaultActor()
		}
		r, err := v.Block(args[0], args[1], by)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "⛔ %s 受阻: %s\n", r.Meta.ID, args[1])
		return nil
	},
}

var unblockCmd = &cobra.Command{
	Use:   "unblock <T 编号>",
	Short: "解除阻塞（blocked → in_progress）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := blockBy
		if by == "" {
			by = defaultActor()
		}
		r, err := v.Unblock(args[0], by)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "▶️  %s 恢复进行\n", r.Meta.ID)
		return nil
	},
}

func init() {
	blockCmd.Flags().StringVar(&blockBy, "by", "", "执行者（默认 $PORT_ACTOR 或 <user>@<host>）")
	unblockCmd.Flags().StringVar(&blockBy, "by", "", "执行者")
	rootCmd.AddCommand(blockCmd, unblockCmd)
}
