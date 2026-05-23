package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func versionString() string {
	return fmt.Sprintf("dotsync %s (built %s, commit %s)\n", version, buildTime, commit)
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
