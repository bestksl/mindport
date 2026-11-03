package command

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bestksl/mindport/internal/server"
	"github.com/bestksl/mindport/internal/store"
)

var (
	pairAddr    string
	pairURLOnly bool
)

type pairTarget struct {
	ip   string
	kind string // 组网 | 局域网 | 地址
}

// pairCmd 为正在运行的 serve 铸一个一次性配对码，打印二维码与各可达地址的
// 配对链接。配对码只与 vault 绑定、与地址无关，所以同一个码可配多条链接。
var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "生成手机/浏览器配对链接与二维码（需 serve 在运行）",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		st, err := server.ReadServeState(v.Root)
		if err != nil {
			return err
		}
		if st == nil {
			return fmt.Errorf("没发现正在运行的 port serve：先在另一个终端启动它（建议加 --auth）")
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", st.Port), time.Second)
		if err != nil {
			return fmt.Errorf("serve 状态在但端口 %s 连不上，可能已退出；请重新启动 port serve", st.Port)
		}
		conn.Close()

		targets := pairCandidates(st, pairAddr)
		if len(targets) == 0 {
			return fmt.Errorf("没有可用于配对的地址：用 --addr <IP> 指定，或确认组网/局域网网卡已连接")
		}

		w := cmd.OutOrStdout()

		path := "/"
		code := ""
		if st.Auth {
			c, _, err := v.IssuePairingCode("")
			if err != nil {
				return err
			}
			code = c
			path = "/pair#code=" + code
			fmt.Fprintf(w, "🔗 配对思港（%d 分钟内有效，仅可使用一次）\n", int(store.PairingTTL.Minutes()))
			fmt.Fprintln(w, "   手机相机扫下面的码；扫不了就打开链接，或在页面里手输配对码")
		} else {
			fmt.Fprintln(w, "ℹ️  serve 未启用访问鉴权：同网设备打开即可，无需配对")
		}

		urls := make([]string, len(targets))
		for i, t := range targets {
			urls[i] = fmt.Sprintf("http://%s:%s%s", t.ip, st.Port, path)
		}
		if !pairURLOnly {
			qr, err := renderQRANSI(urls[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "二维码渲染失败：%v\n", err)
			} else {
				fmt.Fprintln(w, qr)
			}
		}
		for i, u := range urls {
			fmt.Fprintf(w, "   %s %s\n", padLabel(targets[i].kind), u)
		}
		if code != "" {
			fmt.Fprintf(w, "   配对码: %s\n", code)
		}
		if st.Host == "127.0.0.1" || st.Host == "localhost" {
			fmt.Fprintln(w, "   ⚠ serve 当前只监听 127.0.0.1，手机连不上：请用 port serve --host 0.0.0.0 重启")
		}
		return nil
	},
}

// padLabel 让输出里的地址列对齐（中文标签按两字宽补齐）。
func padLabel(s string) string {
	target := 8
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			target -= 2
		} else {
			target--
		}
	}
	if target < 0 {
		target = 0
	}
	return s + ":" + strings.Repeat(" ", target-1)
}

// pairCandidates 给出配对地址的先后顺序：显式 --addr 唯一；否则组网地址优先，
// 再局域网；serve 若绑在某个具体 IP 上，把它提到最前并去重。
func pairCandidates(st *server.ServeState, explicit string) []pairTarget {
	if explicit != "" {
		return []pairTarget{{ip: strings.TrimSpace(explicit), kind: "地址"}}
	}
	var out []pairTarget
	seen := map[string]bool{}
	add := func(ip, kind string) {
		if ip == "" || ip == "0.0.0.0" || ip == "::" || seen[ip] {
			return
		}
		seen[ip] = true
		out = append(out, pairTarget{ip: ip, kind: kind})
	}
	primary, mesh := server.ListenAddresses()
	host := st.Host
	if host != "" && host != "0.0.0.0" && host != "::" && host != "127.0.0.1" && host != "localhost" {
		add(host, "地址")
	}
	for _, ip := range mesh {
		add(ip, "组网")
	}
	for _, ip := range primary {
		add(ip, "局域网")
	}
	return out
}

func init() {
	pairCmd.Flags().StringVar(&pairAddr, "addr", "", "指定配对链接使用的 IP（默认组网地址优先）")
	pairCmd.Flags().BoolVar(&pairURLOnly, "url-only", false, "只打印链接，不渲染二维码")
	rootCmd.AddCommand(pairCmd)
}
