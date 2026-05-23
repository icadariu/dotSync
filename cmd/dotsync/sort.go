package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/config"
)

var sortCmd = &cobra.Command{
	Use:   "sort",
	Short: "Sort entries by src path and renumber IDs",
	Long: "Sort .dotsync.yaml entries by their src path (ascending, case-sensitive)\n" +
		"and renumber IDs sequentially from 1. Useful after manually editing the\n" +
		"config to leave a clean, predictable layout. If nothing would change,\n" +
		"the file is left untouched.",
	Args: cobra.NoArgs,
	RunE: runSort,
}

func init() {
	rootCmd.AddCommand(sortCmd)
}

func runSort(_ *cobra.Command, _ []string) error {
	path := cfgPath()
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	before := snapshotEntries(cfg.Entries)
	cfg.Entries = config.SortEntriesBySrc(cfg.Entries)
	cfg.Entries = config.NormalizeIDs(cfg.Entries)
	after := snapshotEntries(cfg.Entries)

	if entriesEqual(before, after) {
		fmt.Println("no changes")
		return nil
	}

	if err := config.Save(path, cfg); err != nil {
		return err
	}
	fmt.Printf("sorted %d entries\n", len(cfg.Entries))
	return nil
}

// snapshotEntries returns a copy of (ID, Src) pairs in order — enough to
// detect both reordering and ID changes.
func snapshotEntries(entries []config.Entry) []config.Entry {
	out := make([]config.Entry, len(entries))
	copy(out, entries)
	return out
}

func entriesEqual(a, b []config.Entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Src != b[i].Src {
			return false
		}
	}
	return true
}
