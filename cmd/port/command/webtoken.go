package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/server"
)

// webTokenCmd 打印（不存在则生成）网页访问密钥。默认内网免证敞开，只有
// serve --auth（或 PORT_AUTH=1）上锁后，手机首次打开 /login 才需要粘贴它；
// 推荐改用 port pair 扫码配对，密钥登录只是兜底。
var webTokenCmd = &cobra.Command{
	Use:   "web-token",
	Short: "查看/生成网页访问密钥（仅 serve --auth 上锁时需要；推荐用 port pair 配对）",
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
		fmt.Fprintf(w, "🔑 网页访问密钥（%s）：\n%s\n", server.WebTokenPath(v.Root), tok)
		fmt.Fprintln(w, "手机/浏览器打开思港登录页后粘贴；登录签发 30 天有效的设备会话。密钥只存本机 0600 文件，勿外传。")
		fmt.Fprintln(w, "更省事的方式：serve 运行中另开终端执行 port pair，手机扫码即完成配对。")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(webTokenCmd)
}
