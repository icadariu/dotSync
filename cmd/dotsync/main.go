package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/diff"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:               "dotsync",
	Short:             "Manage dotfile symlinks across machines",
	SilenceUsage:      true,
	SilenceErrors:     true,
	PersistentPreRunE: rootGuard,
}

// Test seams for the root guard.
var geteuid = os.Geteuid

var fileOwner = func(path string) (int, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}

// rootGuard refuses to run as root on a config owned by another user, so a
// user-writable config cannot drive root-level filesystem changes.
func rootGuard(cmd *cobra.Command, _ []string) error {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "completion", "__complete", "__completeNoDesc":
			return nil
		}
	}
	if geteuid() != 0 {
		return nil
	}
	path := cfgPath()
	if uid, ok := fileOwner(path); ok && uid != 0 {
		return fmt.Errorf("refusing to run as root on %s owned by uid %d; run as that user or fix the file's owner", path, uid)
	}
	return nil
}

// builtinExamples covers the subcommands cobra creates itself inside Execute.
var builtinExamples = map[string]string{
	"completion": `  # write the zsh script to a directory on fpath (before compinit)
  dotsync completion zsh > ~/.zsh/completions/_dotsync

  # same for bash
  dotsync completion bash > ~/.local/share/bash-completion/completions/dotsync`,
	"help": `  # same as dotsync add --help
  dotsync help add`,
}

func init() {
	defaultHelp := rootCmd.HelpFunc()
	// Examples print last, after Global Flags, instead of cobra's spot above Flags.
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		ex := cmd.Example
		if ex == "" && cmd.Parent() == rootCmd {
			ex = builtinExamples[cmd.Name()]
		}
		base := cmd.Example
		cmd.Example = ""
		defaultHelp(cmd, args)
		cmd.Example = base
		if ex != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nExamples:\n%s\n", ex)
		}
	})
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

// selectEntries returns the entries whose IDs are listed in args, in config
// order; with no args it returns all entries.
func selectEntries(entries []config.Entry, args []string) ([]config.Entry, error) {
	if len(args) == 0 {
		return entries, nil
	}
	want := make(map[int]bool, len(args))
	for _, a := range args {
		id, err := strconv.Atoi(a)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", a, err)
		}
		want[id] = true
	}
	var out []config.Entry
	for _, e := range entries {
		if want[e.ID] {
			out = append(out, e)
			delete(want, e.ID)
		}
	}
	for _, a := range args {
		if id, _ := strconv.Atoi(a); want[id] {
			return nil, fmt.Errorf("no entry with id %d", id)
		}
	}
	return out, nil
}

func main() {
	if HandleVersionFlag() || handleHelpFlag(os.Args[1:]) {
		return
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, errorLine(err))
		os.Exit(1)
	}
}

// handleHelpFlag serves --help/-h before cobra parses flags, so a value-taking
// flag right before it (add --src --help) cannot swallow it as its value.
func handleHelpFlag(args []string) bool {
	if i := slices.Index(args, "--"); i >= 0 {
		args = args[:i]
	}
	if !slices.Contains(args, "--help") && !slices.Contains(args, "-h") {
		return false
	}
	// Execute normally adds these; Find needs them now (both calls are idempotent).
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()
	cmd, _, err := rootCmd.Find(args)
	if err != nil {
		cmd = rootCmd
	}
	_ = cmd.Help()
	return true
}

// errorLine is how main prints a failed command; errors embed config paths,
// so control bytes are escaped like every other line of output.
func errorLine(err error) string {
	return diff.Escape(err.Error())
}
