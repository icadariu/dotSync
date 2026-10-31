package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/diff"
)

var editCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a dotfile entry in $EDITOR",
	Example: `  # edit entry 3 in $EDITOR (vi if unset)
  dotsync edit 3

  # use another editor for one run
  EDITOR=nano dotsync edit 3`,
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

	idx := config.IndexOf(cfg.Entries, id)
	if idx < 0 {
		return fmt.Errorf("no entry with id %d", id)
	}

	edited, err := openEditorForEntry(cfg.Entries[idx])
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determining home directory: %w", err)
	}
	src, err := config.ResolvePath(edited.Src, home)
	if err != nil {
		return fmt.Errorf("src: %w", err)
	}
	dst, err := config.ResolvePath(edited.Dst, home)
	if err != nil {
		return fmt.Errorf("dst: %w", err)
	}
	dst, cancelled, err := resolveDstRule(edited.Dst, dst, src, false)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Println("cancelled")
		return nil
	}
	edited.Src, edited.Dst = src, dst
	if ok, err := confirmHomeDst(edited.Dst, home, false); !ok || err != nil {
		return err
	}

	if _, err := os.Stat(edited.Src); err != nil {
		return fmt.Errorf("src does not exist after edit: %w", err)
	}
	if err := config.ValidateEntry(cfg.Entries, edited, idx); err != nil {
		return err
	}

	old := cfg.Entries[idx]
	edited.ID = old.ID // preserve original id
	cfg.Entries[idx] = edited
	if err := config.Save(cfgPath(), cfg); err != nil {
		return err
	}
	fmt.Printf("updated %d: %s -> %s\n", edited.ID, diff.Escape(edited.Src), diff.Escape(edited.Dst))

	if old.Dst == edited.Dst {
		return nil
	}
	own, why, err := ownLink(old)
	if err != nil {
		return fmt.Errorf("entry %d updated in config, but %w", old.ID, err)
	}
	switch {
	case own:
		if err := os.Remove(old.Dst); err != nil {
			return fmt.Errorf("entry %d updated in config, but could not unlink %s: %w", old.ID, old.Dst, err)
		}
		fmt.Printf("unlinked %s\n", diff.Escape(old.Dst))
	case why != "":
		fmt.Printf("%s %s — left in place\n", diff.Escape(old.Dst), diff.Escape(why))
	}
	return nil
}
