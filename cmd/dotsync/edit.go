package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/config"
)

var editCmd = &cobra.Command{
	Use:               "edit <id>",
	Short:             "Edit a dotfile entry in $EDITOR",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeEntryIDs,
	RunE:              runEdit,
}

func init() {
	rootCmd.AddCommand(editCmd)
}

func runEdit(_ *cobra.Command, args []string) error {
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", args[0], err)
	}
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}

	idx := -1
	for i, e := range cfg.Entries {
		if e.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("no entry with id %d", id)
	}

	edited, err := openEditorForEntry(cfg.Entries[idx])
	if err != nil {
		return err
	}

	if _, err := os.Stat(edited.Src); err != nil {
		return fmt.Errorf("src does not exist after edit: %w", err)
	}
	for i, e := range cfg.Entries {
		if i != idx && e.Dst == edited.Dst {
			return fmt.Errorf("entry %d already uses dst %s", e.ID, edited.Dst)
		}
	}

	edited.ID = cfg.Entries[idx].ID // preserve original id
	cfg.Entries[idx] = edited
	return config.Save(cfgPath(), cfg)
}
