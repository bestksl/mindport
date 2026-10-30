package command

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/server"
)

// permMcpCmd 是隐藏的内部命令：网页桥起的 claude 子进程通过 --mcp-config
// 把它当 stdio MCP server 拉起；它把 check_permission 回调经 unix socket
// 转给正在 serve 的思港主进程，阻塞等手机端拍板。人不直接调用它。
var permMcpCmd = &cobra.Command{
	Use:    "_perm-mcp",
	Short:  "内部：手机工具审批的 stdio MCP 服务（由桥自动拉起，勿手敲）",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// stdout 只能吐 JSON-RPC，任何日志都走 cobra 的 stderr。
		return server.RunPermMCP(os.Stdin, cmd.OutOrStdout(), os.Getenv, nil)
	},
}

func init() {
	rootCmd.AddCommand(permMcpCmd)
}
