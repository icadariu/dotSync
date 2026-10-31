package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/diff"
	"github.com/icadariu/dotSync/internal/linker"
	"github.com/icadariu/dotSync/internal/prompt"
)

var unlinkCmd = &cobra.Command{
	Use:   "unlink [id...]",
	Short: "Remove managed symlinks (optionally restore backups)",
	Example: `  # remove every managed symlink (config and src stay as they are)
  dotsync unlink

  # remove the symlinks of entries 2 and 5 without asking
  dotsync unlink --force 2 5

  # remove the symlink and move the backup back into place
  dotsync unlink --restore 2`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeEntryIDsMulti,
	RunE:              runUnlink,
}

var unlinkForce bool
var unlinkRestore bool

func init() {
	rootCmd.AddCommand(unlinkCmd)
	unlinkCmd.Flags().BoolVar(&unlinkForce, "force", false, "skip confirmation prompt")
	unlinkCmd.Flags().BoolVar(&unlinkRestore, "restore", false, "move the backup back into place")
}

func runUnlink(_ *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}
	entries, err := selectEntries(cfg.Entries, args)
	if err != nil {
		return err
	}

	if !unlinkForce {
		ch, err := prompt.Confirm(
			fmt.Sprintf("unlink %d entries?", len(entries)),
			[]rune{'y', 'n'}, 'n',
		)
		if err != nil {
			return err
		}
		if ch == 'n' {
			fmt.Println("cancelled")
			return nil
		}
	}

	failed := 0
	for _, e := range entries {
		if err := unlinkEntry(e, cfg.BackupSuffix, unlinkRestore); err != nil {
			fmt.Fprintln(os.Stderr, errorLine(err))
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d entries failed", failed, len(entries))
	}
	return nil
}

func unlinkEntry(e config.Entry, suffix string, restore bool) error {
	own, why, err := ownLink(e)
	if err != nil {
		return err
	}
	if !own {
		if why == "" {
			fmt.Printf("%s not found\n", diff.Escape(e.Dst))
		} else {
			fmt.Printf("%s is not our symlink, left in place\n", diff.Escape(e.Dst))
		}
		return nil
	}
	if err := os.Remove(e.Dst); err != nil {
		return fmt.Errorf("could not unlink %s: %w", e.Dst, err)
	}
	fmt.Printf("unlinked %s\n", diff.Escape(e.Dst))
	if !restore {
		return nil
	}
	backup := e.Dst + suffix
	fi, err := os.Lstat(backup)
	if err != nil {
		return nil
	}
	if linker.ForeignOwned(fi) {
		return fmt.Errorf("backup %s is owned by another user, not restoring", backup)
	}
	if err := os.Rename(backup, e.Dst); err != nil {
		return fmt.Errorf("could not restore %s: %w", backup, err)
	}
	fmt.Printf("restored %s -> %s\n", diff.Escape(backup), diff.Escape(e.Dst))
	return nil
}
