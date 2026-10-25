// Package command 实现 port 的 cobra 子命令。
package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bestksl/mindport/internal/store"
	"github.com/spf13/cobra"
)

var vaultFlag string

var rootCmd = &cobra.Command{
	Use:   "port",
	Short: "思港 MindPort — 人与 AI 共用的本地任务调度港",
	Long: "思港 MindPort：念头入港停泊，任务等待调度，agent 领走出港、做完归港。\n" +
		"数据是 vault 目录里的 Markdown 文件（ADR-0003）。",
}

// Execute 是命令行入口。
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&vaultFlag, "vault", "",
		"vault 目录（默认 $PORT_VAULT，再退到 ~/MindPort）")
}

func openVault() (*store.Vault, error) {
	p := vaultFlag
	if p == "" {
		p = os.Getenv("PORT_VAULT")
	}
	if p == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		p = filepath.Join(home, "MindPort")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	return store.Open(abs)
}

func defaultActor() string {
	if a := os.Getenv("PORT_ACTOR"); a != "" {
		return a
	}
	u, err := os.UserHomeDir()
	_ = err
	name := filepath.Base(u)
	if hn, err := os.Hostname(); err == nil {
		return fmt.Sprintf("%s@%s", name, hn)
	}
	return name + "@studio"
}
