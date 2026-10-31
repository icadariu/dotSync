package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/icadariu/dotSync/internal/config"
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

	// Load renumbers IDs, so check the raw file for bad ones.
	idsBad := false
	if data, err := os.ReadFile(path); err == nil {
		var raw struct {
			Entries []struct {
				ID int `yaml:"id"`
			} `yaml:"entries"`
		}
		if yaml.Unmarshal(data, &raw) == nil {
			for i, e := range raw.Entries {
				if e.ID != i+1 {
					idsBad = true
				}
			}
		}
	}

	before := slices.Clone(cfg.Entries)
	cfg.Entries = config.SortEntriesBySrc(cfg.Entries)
	cfg.Entries = config.NormalizeIDs(cfg.Entries)

	if !idsBad && slices.EqualFunc(before, cfg.Entries, func(a, b config.Entry) bool {
		return a.ID == b.ID && a.Src == b.Src && a.Dst == b.Dst
	}) {
		fmt.Println("no changes")
		return nil
	}

	if err := config.Save(path, cfg); err != nil {
		return err
	}
	fmt.Printf("sorted %d entries\n", len(cfg.Entries))
	return nil
}
