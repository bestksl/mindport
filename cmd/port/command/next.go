package command

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/store"
)

var nextJSON bool

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "输出当前最重要的 ready 任务（agent 拉活入口）",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.Next()
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				fmt.Fprintln(cmd.ErrOrStderr(), "港里没有待领取的任务")
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
		fmt.Fprintf(cmd.OutOrStdout(), "领取: port claim %s\n查看: port show %s\n", r.Meta.ID, r.Meta.ID)
		return nil
	},
}

func init() {
	nextCmd.Flags().BoolVar(&nextJSON, "json", false, "输出 JSON")
	rootCmd.AddCommand(nextCmd)
}
