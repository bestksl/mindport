package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// migrateInboxCmd 一次性迁移：旧 inbox 闪念 → 草稿任务。部署新版本时执行一次。
var migrateInboxCmd = &cobra.Command{
	Use:   "migrate-inbox",
	Short: "把旧 inbox/ 里的闪念批量转成草稿任务（一次性迁移）",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		n, err := v.MigrateLegacyInbox()
		if err != nil {
			return err
		}
		if n == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "没有需要迁移的旧闪念（inbox/ 为空或不存在）")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "已把 %d 条旧闪念转成草稿任务；原件移到 archive/inbox-legacy/\n", n)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(migrateInboxCmd)
}
