package main

import (
	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/linker"
)

var applyCmd = &cobra.Command{
	Use:               "apply [id...]",
	Short:             "Apply dotfile symlinks",
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeEntryIDsMulti,
	RunE:              runApply,
}

var applyForce bool
var applyNoBackup bool

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVarP(&applyForce, "force", "f", false, "skip per-entry confirmation prompts")
	applyCmd.Flags().BoolVar(&applyNoBackup, "no-backup", false, "delete conflicting dst instead of renaming to .bk")
	flagExample(applyCmd.Flags(), "force",
		"  # apply every entry without per-entry prompts\n  dotsync apply --force")
	flagExample(applyCmd.Flags(), "no-backup",
		"  # delete conflicting files instead of renaming them to <dst><suffix>\n  dotsync apply --no-backup 3")
}

func runApply(_ *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}
	entries, err := selectEntries(cfg.Entries, args)
	if err != nil {
		return err
	}
	return linker.Apply(entries, linker.ApplyOptions{
		BackupSuffix: cfg.BackupSuffix,
		Force:        applyForce,
		NoBackup:     applyNoBackup,
	})
}
