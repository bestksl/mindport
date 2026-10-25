package command

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/server"
)

var (
	serveHost string
	servePort int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 Web 服务（手机浏览器/PWA 与 JSON API）",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		host := envOr("PORT_HOST", serveHost)
		port := envOr("PORT_PORT", fmt.Sprint(servePort))
		addr := net.JoinHostPort(host, port)

		srv := &http.Server{Addr: addr, Handler: (&server.Server{V: v}).Routes()}

		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "⚓ 思港港口已开\n")
		fmt.Fprintf(w, "   本机:   http://127.0.0.1:%s\n", port)
		fmt.Fprintf(w, "   vault:  %s\n", v.Root)
		for _, ip := range lanIPs() {
			fmt.Fprintf(w, "   局域网: http://%s:%s\n", ip, port)
		}
		if host == "127.0.0.1" {
			fmt.Fprintln(w, "   （仅本机能访问；手机同 WiFi 访问请加 --host 0.0.0.0）")
		}
		return srv.ListenAndServe()
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveHost, "host", "127.0.0.1", "监听地址（0.0.0.0 允许局域网访问）")
	serveCmd.Flags().IntVar(&servePort, "port", 8787, "监听端口")
	rootCmd.AddCommand(serveCmd)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// lanIPs 通过 ipconfig 取常见物理网卡地址（macOS）。
func lanIPs() []string {
	var out []string
	for _, iface := range []string{"en0", "en1", "en2"} {
		b, err := exec.Command("ipconfig", "getifaddr", iface).Output()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(b))
		if ip != "" {
			out = append(out, ip)
		}
	}
	return out
}
