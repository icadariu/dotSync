package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/config"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all dotfile entries",
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSRC\tDST")
	for _, e := range cfg.Entries {
		fmt.Fprintf(w, "%d\t%s\t%s\n", e.ID, e.Src, e.Dst)
	}
	return w.Flush()
}
