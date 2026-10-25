package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	logBy   string
	logQuiet bool
)

var logCmd = &cobra.Command{
	Use:   "log <T 编号> <内容>",
	Short: "追加一条过程日志（agent 同步进展的主要方式）",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := logBy
		if by == "" {
			by = defaultActor()
		}
		r, err := v.Log(args[0], args[1], by)
		if err != nil {
			return err
		}
		if !logQuiet {
			fmt.Fprintf(cmd.OutOrStdout(), "📝 %s 已记录日志\n", r.Meta.ID)
		}
		return nil
	},
}

func init() {
	logCmd.Flags().StringVar(&logBy, "by", "", "记录者（默认 $PORT_ACTOR 或 <user>@<host>）")
	logCmd.Flags().BoolVar(&logQuiet, "quiet", false, "只写不输出")
	rootCmd.AddCommand(logCmd)
}
