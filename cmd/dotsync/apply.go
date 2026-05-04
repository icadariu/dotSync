package main

import (
	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/config"
	"github.com/icadariu/dotsync/internal/linker"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply dotfile symlinks",
	RunE:  runApply,
}

var applyForce bool
var applyNoBackup bool

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVarP(&applyForce, "force", "f", false, "skip per-entry confirmation prompts")
	applyCmd.Flags().BoolVar(&applyNoBackup, "no-backup", false, "delete conflicting dst instead of renaming to .bk")
}

func runApply(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}
	return linker.Apply(cfg.Entries, linker.ApplyOptions{
		BackupSuffix: cfg.BackupSuffix,
		Force:        applyForce,
		NoBackup:     applyNoBackup,
	})
}
