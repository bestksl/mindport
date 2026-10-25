package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var inboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "列出 inbox 里尚未整理的闪念",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := openVault()
		if err != nil {
			return err
		}
		thoughts, err := v.ListThoughts()
		if err != nil {
			return err
		}
		w := cmd.OutOrStdout()
		if len(thoughts) == 0 {
			fmt.Fprintln(w, "inbox 空空如也")
			return nil
		}
		for _, t := range thoughts {
			fmt.Fprintf(w, "%s  %s  (%s)\n",
				t.Meta.Created.Format("01-02 15:04"),
				t.Meta.Title,
				filepath.Base(t.Path))
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(inboxCmd) }
