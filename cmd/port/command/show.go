package command

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var showJSON bool

var showCmd = &cobra.Command{
	Use:   "show <T 编号>",
	Short: "查看任务全文（描述/验收/日志），agent 领取后读完整上下文",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		r, err := v.GetTask(args[0])
		if err != nil {
			return err
		}
		w := cmd.OutOrStdout()
		if showJSON {
			return json.NewEncoder(w).Encode(struct {
				Meta interface{} `json:"meta"`
				Body string      `json:"body"`
				Path string      `json:"path"`
			}{r.Meta, r.Body, r.Path})
		}
		goal := "-"
		if r.Meta.Goal != "" {
			goal = r.Meta.Goal
		}
		fmt.Fprintf(w, "%s  %s  [%s]  %s\n", r.Meta.ID, statusZH(r.Meta.Status), impZH(r.Meta.Importance), r.Meta.Title)
		fmt.Fprintf(w, "归属: %s    目标: %s\n", assignee(r.Meta), goal)
		fmt.Fprintln(w, "----------------------------------------")
		fmt.Fprintln(w, r.Body)
		return nil
	},
}

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "输出 JSON（meta + body）")
	rootCmd.AddCommand(showCmd)
}
