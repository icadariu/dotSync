package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/config"
)

var addCmd = &cobra.Command{
	Use:   "add [src dst]",
	Short: "Add a dotfile entry",
	Args:  cobra.RangeArgs(0, 2),
	RunE:  runAdd,
}

var addSrc, addDest string

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVar(&addSrc, "src", "", "source path in dotfiles repo")
	addCmd.Flags().StringVar(&addDest, "dest", "", "destination path in home directory")
	_ = addCmd.RegisterFlagCompletionFunc("src", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveDefault
	})
	_ = addCmd.RegisterFlagCompletionFunc("dest", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveDefault
	})
}

func runAdd(cmd *cobra.Command, args []string) error {
	src := addSrc
	dst := addDest
	if !cmd.Flags().Changed("src") && len(args) >= 1 {
		src = args[0]
	}
	if !cmd.Flags().Changed("dest") && len(args) >= 2 {
		dst = args[1]
	}
	if src == "" {
		return fmt.Errorf("src is required: use --src or pass it as the first argument")
	}
	if dst == "" {
		return fmt.Errorf("dest is required: use --dest or pass it as the second argument")
	}

	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determining home directory: %w", err)
	}
	resolvedSrc, err := config.ResolveSrcPath(src)
	if err != nil {
		return fmt.Errorf("src: %w", err)
	}
	resolvedDst := config.ResolveDstPath(dst, home)

	if _, err := os.Stat(resolvedSrc); err != nil {
		return fmt.Errorf("src does not exist: %w", err)
	}
	for _, e := range cfg.Entries {
		if e.Dst == resolvedDst {
			return fmt.Errorf("entry %d already uses dst %s", e.ID, resolvedDst)
		}
	}
	cfg.Entries = append(cfg.Entries, config.Entry{
		Src: resolvedSrc,
		Dst: resolvedDst,
	})
	cfg.Entries = config.NormalizeIDs(cfg.Entries)
	return config.Save(cfgPath(), cfg)
}
