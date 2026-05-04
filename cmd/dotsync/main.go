package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/config"
)

// exitErr signals a specific exit code without printing an error message.
type exitErr struct{ code int }

func (e exitErr) Error() string { return "" }

var configPath string

var rootCmd = &cobra.Command{
	Use:           "dotsync",
	Short:         "Manage dotfile symlinks across machines",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "config file path (default ~/.dotsync.yaml, env: DOTSYNC_CONFIG)")
}

func cfgPath() string {
	if configPath != "" {
		return configPath
	}
	if env := os.Getenv("DOTSYNC_CONFIG"); env != "" {
		return env
	}
	path, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: cannot determine home directory:", err)
		return ".dotsync.yaml"
	}
	return path
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		var ee exitErr
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
