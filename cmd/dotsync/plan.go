package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/color"
	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/diff"
	"github.com/icadariu/dotSync/internal/linker"
)

var planCmd = &cobra.Command{
	Use:               "plan [id...]",
	Short:             "Show what apply would do (dry-run with diffs)",
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeEntryIDsMulti,
	RunE:              runPlan,
}

var (
	planVerbose bool
	planNoPager bool
)

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.Flags().BoolVarP(&planVerbose, "verbose", "v", false, "show unchanged entries")
	planCmd.Flags().BoolVar(&planNoPager, "no-pager", false, "do not pipe output through a pager")
}

func runPlan(_ *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}
	entries, err := selectEntries(cfg.Entries, args)
	if err != nil {
		return err
	}
	results := linker.Plan(entries, cfg.BackupSuffix)
	var buf bytes.Buffer
	n, changes := 0, 0
	for _, r := range results {
		id := r.Entry.ID
		switch r.Status {
		case linker.StatusOK:
			if planVerbose {
				fmt.Fprintf(&buf, "[%d] %s  %s\n", id, "= unchanged", diff.Escape(r.Entry.Dst))
			}
		case linker.StatusLink:
			changes++
			fmt.Fprintf(&buf, "[%d] %s     %s -> %s\n", id, color.Green("+ create"), diff.Escape(r.Entry.Dst), diff.Escape(r.Entry.Src))
			if r.Message != "" {
				fmt.Fprint(&buf, color.Diff(r.Message))
			}
		case linker.StatusRelink:
			changes++
			fmt.Fprintf(&buf, "[%d] %s     %s\n", id, color.Yellow("~ relink"), diff.Escape(r.Entry.Dst))
			fmt.Fprintf(&buf, "    %s → %s\n", diff.Escape(r.OldTarget), diff.Escape(r.Entry.Src))
			if r.Message != "" {
				fmt.Fprint(&buf, color.Diff(r.Message))
			}
		case linker.StatusConflict:
			changes++
			fmt.Fprintf(&buf, "[%d] %s    %s\n", id, color.Yellow("~ replace"), diff.Escape(r.Entry.Dst))
			if r.Message != "" {
				fmt.Fprint(&buf, color.Diff(r.Message))
			}
		case linker.StatusError:
			fmt.Fprintf(os.Stderr, "[%d] %s      %s: %s\n", id, color.Red("! error"), diff.Escape(r.Entry.Dst), diff.Escape(r.Message))
			n++
		}
	}
	if n == 0 && changes == 0 && !planVerbose {
		fmt.Fprintln(&buf, "no changes")
	}
	if changes > 0 && !planNoPager && stdoutIsTTY() && pagerCommand() != "" {
		page(buf.Bytes())
	} else {
		os.Stdout.Write(buf.Bytes())
	}
	if n > 0 {
		return fmt.Errorf("%d of %d entries have errors", n, len(results))
	}
	return nil
}
