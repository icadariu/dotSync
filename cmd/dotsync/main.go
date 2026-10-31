package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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

const exampleAnnotation = "dotsync_example"

// flagExample attaches help examples to a flag; --help shows them, filtered to the flags passed.
func flagExample(fs *pflag.FlagSet, name string, ex ...string) {
	_ = fs.SetAnnotation(name, exampleAnnotation, ex)
}

func init() {
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		var all, picked []string
		cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
			ex := f.Annotations[exampleAnnotation]
			all = append(all, ex...)
			if f.Changed {
				picked = append(picked, ex...)
			}
		})
		if len(picked) == 0 {
			picked = all
		}
		base := cmd.Example
		if len(picked) > 0 {
			cmd.Example = strings.Join(picked, "\n\n")
		}
		defaultHelp(cmd, args)
		cmd.Example = base
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
	if HandleVersionFlag() {
		return
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, errorLine(err))
		os.Exit(1)
	}
}

// errorLine is how main prints a failed command; errors embed config paths,
// so control bytes are escaped like every other line of output.
func errorLine(err error) string {
	return diff.Escape(err.Error())
}
