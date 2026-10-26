package command

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/store"
)

var (
	nextJSON bool
	nextBy   string
)

var nextCmd = &cobra.Command{
	Use:   "next [--by 船名]",
	Short: "输出派给本船、待接令的最重要任务（agent pull 入口；锚地货不可见）",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		by := nextBy
		if by == "" {
			by = defaultActor()
		}
		r, err := v.Next(by)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				fmt.Fprintf(cmd.ErrOrStderr(), "没有派给 %s 的待接令任务\n", by)
				return nil
			}
			return err
		}
		if nextJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(r.Meta)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s  [%s]  %s\n", r.Meta.ID, impZH(r.Meta.Importance), r.Meta.Title)
		if r.Meta.Goal != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "目标: %s\n", r.Meta.Goal)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "接令: port claim %s\n查看: port show %s\n", r.Meta.ID, r.Meta.ID)
		return nil
	},
}

func init() {
	nextCmd.Flags().BoolVar(&nextJSON, "json", false, "输出 JSON")
	nextCmd.Flags().StringVar(&nextBy, "by", "", "船名（默认 $PORT_ACTOR 或 <user>@<host>）")
	rootCmd.AddCommand(nextCmd)
}
