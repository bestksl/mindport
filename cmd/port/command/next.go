package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/store"
)

var (
	nextJSON bool
	nextBy   string
	nextCwd  string
)

var nextCmd = &cobra.Command{
	Use:   "next [--by 执行者]",
	Short: "输出指派给我、待领取的最重要任务（agent pull 入口；待命区任务不可见）",
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
		cwd := nextCwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		r, err := v.Next(by, cwd)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				fmt.Fprintln(cmd.ErrOrStderr(), "没有指派给本工作目录（当前项目目录）的待领取任务")
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
	nextCmd.Flags().StringVar(&nextBy, "by", "", "执行者（默认 $PORT_ACTOR 或 <user>@<host>）")
	nextCmd.Flags().StringVar(&nextCwd, "cwd", "", "工作目录/项目目录（默认当前目录；只看指派给本工作目录的任务）")
	rootCmd.AddCommand(nextCmd)
}
