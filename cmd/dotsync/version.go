package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func versionString() string {
	return fmt.Sprintf("dotsync %s (commit %s, built %s)\n", version, commit, buildDate)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Print(versionString())
	},
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate(versionString())
	rootCmd.AddCommand(versionCmd)
}
