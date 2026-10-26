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
	Short: "列任务（按重要程度排序；可过滤状态，--json 供 agent）",
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
			fmt.Fprintf(w, "%s  %s[%-2s]  %s  %-18s  %s\n",
				t.Meta.ID, impMark(t.Meta.Importance), impZH(t.Meta.Importance),
				statusZH(t.Meta.Status), assignee(t.Meta), t.Meta.Title)
		}
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "", "过滤状态：draft/anchored/ready/in_progress/blocked/done")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "输出 JSON")
	rootCmd.AddCommand(listCmd)
}
