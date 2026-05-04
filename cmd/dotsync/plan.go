package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/color"
	"github.com/icadariu/dotsync/internal/config"
	"github.com/icadariu/dotsync/internal/linker"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show what apply would do (dry-run with diffs)",
	RunE:  runPlan,
}

var planVerbose bool

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.Flags().BoolVarP(&planVerbose, "verbose", "v", false, "show unchanged entries")
}

func runPlan(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}
	results := linker.Plan(cfg.Entries)
	for _, r := range results {
		id := r.Entry.ID
		switch r.Status {
		case linker.StatusOK:
			if planVerbose {
				fmt.Printf("[%d] %s  %s\n", id, "= unchanged", r.Entry.Dst)
			}
		case linker.StatusLink:
			fmt.Printf("[%d] %s     %s -> %s\n", id, color.Green("+ create"), r.Entry.Dst, r.Entry.Src)
			if r.Message != "" {
				fmt.Print(color.Diff(r.Message))
			}
		case linker.StatusRelink:
			fmt.Printf("[%d] %s     %s\n", id, color.Yellow("~ relink"), r.Entry.Dst)
			fmt.Printf("    %s → %s\n", r.OldTarget, r.Entry.Src)
			if r.Message != "" {
				fmt.Print(color.Diff(r.Message))
			}
		case linker.StatusConflict:
			fmt.Printf("[%d] %s    %s\n", id, color.Yellow("~ replace"), r.Entry.Dst)
			if r.Message != "" {
				fmt.Print(color.Diff(r.Message))
			}
		case linker.StatusError:
			fmt.Fprintf(os.Stderr, "[%d] %s      %s: %s\n", id, color.Red("! error"), r.Entry.Dst, r.Message)
		}
	}
	return nil
}
