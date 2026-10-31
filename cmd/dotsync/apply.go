package main

import (
	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/linker"
)

var applyCmd = &cobra.Command{
	Use:   "apply [id...]",
	Short: "Apply dotfile symlinks",
	Example: `  # link every entry, asking before a conflicting dst is moved to a backup
  dotsync apply

  # apply only entries 2 and 5
  dotsync apply 2 5

  # apply every entry without per-entry prompts
  dotsync apply --force

  # delete a conflicting dst instead of renaming it to <dst><backup_suffix>
  dotsync apply --no-backup 3`,
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
