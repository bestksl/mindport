package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/server"
)

// webTokenCmd 打印（不存在则生成）网页入港证。serve 绑 0.0.0.0 后整个调度台
// 含会话 transcript 都在密钥后面（ADR-0008），手机首次打开 /login 粘贴它。
var webTokenCmd = &cobra.Command{
	Use:   "web-token",
	Short: "查看/生成网页入港证（Web 登录密钥）",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		tok, err := server.EnsureWebToken(v.Root)
		if err != nil {
			return err
		}
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "🔑 网页入港证（%s）：\n%s\n", server.WebTokenPath(v.Root), tok)
		fmt.Fprintln(w, "手机/浏览器打开思港后粘贴登录；30 天内免再输。密钥只存本机 0600 文件，勿外传。")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(webTokenCmd)
}
