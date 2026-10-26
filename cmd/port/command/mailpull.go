package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// mailPull 由 UserPromptSubmit 钩子调用：把调度长在网页上、趁会话忙碌时留的言
// 幂等拉出（PullMail 原子回标 delivered），以 additionalContext 注入这一轮提交。
// 没有留言时静默（空输出 = 钩子无附加）。详见 ADR-0008。
var mailPullCmd = &cobra.Command{
	Use:    "mail-pull",
	Short:  "拉取调度长给本会话的留言（UserPromptSubmit 钩子用）",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		p := readHookStdin()
		if p == nil || p.SessionID == "" {
			return nil // 非钩子环境：无事可做
		}
		pending, err := v.PullMail(p.SessionID)
		if err != nil || len(pending) == 0 {
			return err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "【思港 MindPort · 调度长留言 %d 条（你忙碌时通过调度台网页口授）】\n", len(pending))
		for i, m := range pending {
			fmt.Fprintf(&b, "\n%d.（%s）\n%s\n", i+1, m.TS.Format("01-02 15:04"), strings.TrimSpace(m.Text))
		}
		b.WriteString("\n请把这些留言当作调度长本轮的补充指令处理；若与当前任务冲突，先说明再继续。")

		event := p.HookEventName
		if event == "" {
			event = "UserPromptSubmit"
		}
		out := map[string]any{
			"hookSpecificOutput": map[string]string{
				"hookEventName":     event,
				"additionalContext": b.String(),
			},
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		return enc.Encode(out)
	},
}

func init() {
	rootCmd.AddCommand(mailPullCmd)
}
