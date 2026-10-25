package command

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	listStatus string
	listJSON   bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "列出任务（可按状态过滤，--json 供 agent 解析）",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		tasks, err := v.ListTasks(listStatus)
		if err != nil {
			return err
		}
		w := cmd.OutOrStdout()
		if listJSON {
			metas := make([]interface{}, 0, len(tasks))
			for _, t := range tasks {
				metas = append(metas, t.Meta)
			}
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(metas)
		}
		if len(tasks) == 0 {
			fmt.Fprintln(w, "港里没有匹配的任务")
			return nil
		}
		for _, t := range tasks {
			fmt.Fprintf(w, "%s  %s  %-18s  %s\n",
				t.Meta.ID, statusZH(t.Meta.Status), assignee(t.Meta), t.Meta.Title)
		}
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "", "过滤状态：ready/in_progress/blocked/done")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "输出 JSON")
	rootCmd.AddCommand(listCmd)
}
