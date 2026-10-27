package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/server"
)

// webTokenCmd 打印（不存在则生成）网页入港证。默认内网免证敞开，只有
// serve --auth（或 PORT_AUTH=1）上锁后，手机首次打开 /login 才需要粘贴它。
var webTokenCmd = &cobra.Command{
	Use:   "web-token",
	Short: "查看/生成网页入港证（仅 serve --auth 上锁时需要）",
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
