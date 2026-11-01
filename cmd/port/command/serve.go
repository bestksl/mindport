package command

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/server"
)

var (
	serveHost string
	servePort int
	serveAuth bool
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
		authWanted := serveAuth || os.Getenv("PORT_AUTH") == "1"
		addr := net.JoinHostPort(host, port)

		// 默认内网免证敞开；--auth（或 PORT_AUTH=1）才强制入港证（ADR-0008）：
		// 会话船台能看到 transcript（含命令/代码/思考），不可信网络加这把锁。
		srvHandler := server.New(v)
		if authWanted {
			if err := srvHandler.EnableAuth(); err != nil {
				return err
			}
		}
		// 手机工具审批（v0.9）：默认开，桥养的 claude 遇写操作先在手机上请批；
		// PORT_PERM=0 退回 v0.8 的 acceptEdits 放手模式。起不来不拖垮港口。
		permWanted := os.Getenv("PORT_PERM") != "0"
		permOK := false
		if permWanted {
			if err := srvHandler.EnablePerms(); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  手机审批通道没起来（船队将以 acceptEdits 运行）：%v\n", err)
			} else {
				permOK = true
			}
		}

		// 生产加固：限制读慢/空闲连接；WriteTimeout 必须为 0（SSE 是长连接流式响应）。
		srv := &http.Server{
			Addr:              addr,
			Handler:           srvHandler.Routes(),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		errCh := make(chan error, 1)
		go func() { errCh <- srv.ListenAndServe() }()

		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "⚓ 思港港口已开\n")
		fmt.Fprintf(w, "   本机:   http://127.0.0.1:%s\n", port)
		fmt.Fprintf(w, "   vault:  %s\n", v.Root)
		for _, ip := range lanIPs() {
			fmt.Fprintf(w, "   局域网: http://%s:%s\n", ip, port)
		}
		if authWanted {
			fmt.Fprintln(w, "   入港证: 已启用——浏览器首次打开需粘贴 port web-token 取得的密钥")
		} else {
			fmt.Fprintln(w, "   入港证: 未启用（内网免证敞开；不可信网络请加 --auth 上锁）")
			if host != "127.0.0.1" && host != "localhost" {
				fmt.Fprintln(w, "   ⚠ 0.0.0.0 无鉴权：同网段任何人都能读全部任务与会话 transcript")
			}
		}
		if host == "127.0.0.1" {
			fmt.Fprintln(w, "   （仅本机能访问；手机同 WiFi 访问请加 --host 0.0.0.0）")
		}
		if permOK {
			fmt.Fprintln(w, "   工具审批: 已开启——默认 Auto（低风险自动干、危险操作手机拍板），会话页可切 Plan / 无人值守")
		} else if !permWanted {
			fmt.Fprintln(w, "   工具审批: 关闭（PORT_PERM=0；船队以 acceptEdits 运行）")
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case err := <-errCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case sig := <-sigCh:
			fmt.Fprintf(w, "\n收到 %s，优雅停港（最多等 10s）…\n", sig)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				return err
			}
			return nil
		}
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveHost, "host", "127.0.0.1", "监听地址（0.0.0.0 允许局域网访问）")
	serveCmd.Flags().IntVar(&servePort, "port", 8787, "监听端口")
	serveCmd.Flags().BoolVar(&serveAuth, "auth", false, "强制入港证鉴权（不可信网络用；密钥 port web-token）")
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
