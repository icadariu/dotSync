package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/diff"
	"github.com/icadariu/dotSync/internal/prompt"
)

var addCmd = &cobra.Command{
	Use:   "add [src dst]",
	Short: "Add a dotfile entry",
	Args:  cobra.RangeArgs(0, 2),
	RunE:  runAdd,
}

var addSrc, addDest string
var addAdopt, addForce bool

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVar(&addSrc, "src", "", "source path in dotfiles repo")
	addCmd.Flags().StringVar(&addDest, "dest", "", "destination path in home directory")
	addCmd.Flags().BoolVar(&addAdopt, "adopt", false, "move the existing dst into src, then link dst to it")
	addCmd.Flags().BoolVar(&addForce, "force", false, "skip safety confirmations (a warning is still printed)")
	flagExample(addCmd.Flags(), "src",
		"  # flags instead of positional args (they can be mixed)\n  dotsync add --src ~/dotfiles/.zshrc --dest ~/.zshrc")
	flagExample(addCmd.Flags(), "dest",
		"  # link into a directory: a trailing / keeps the basename of src\n  dotsync add ~/dotfiles/nvim --dest ~/.config/")
	flagExample(addCmd.Flags(), "adopt",
		"  # ~/.zshrc exists but is not tracked yet: move it into the repo, then link it back\n  dotsync add --adopt ~/dotfiles/.zshrc ~/.zshrc")
	flagExample(addCmd.Flags(), "force",
		"  # rare: use $HOME itself as dst without the confirmation (a warning is still printed)\n  dotsync add --force ~/dotfiles/home ~")
	_ = addCmd.RegisterFlagCompletionFunc("src", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveDefault
	})
	_ = addCmd.RegisterFlagCompletionFunc("dest", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveDefault
	})
}

func runAdd(cmd *cobra.Command, args []string) error {
	// Positional args fill whichever of src/dst the flags left unset, in order.
	src, dst, rest := addSrc, addDest, args
	if !cmd.Flags().Changed("src") && len(rest) > 0 {
		src, rest = rest[0], rest[1:]
	}
	if !cmd.Flags().Changed("dest") && len(rest) > 0 {
		dst, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 {
		return fmt.Errorf("unexpected extra arguments: %q", rest)
	}
	if src == "" {
		return fmt.Errorf("src is required: use --src or pass it as the first argument")
	}
	if dst == "" {
		return fmt.Errorf("dest is required: use --dest or pass it as the second argument")
	}

	cfg, err := config.Load(cfgPath())
	if errors.Is(err, fs.ErrNotExist) {
		cfg, err = &config.Config{Version: 1, BackupSuffix: ".bk"}, nil // first add creates the config
	}
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determining home directory: %w", err)
	}
	resolvedSrc, err := config.ResolvePath(src, home)
	if err != nil {
		return fmt.Errorf("src: %w", err)
	}
	resolvedDst, err := config.ResolvePath(dst, home)
	if err != nil {
		return fmt.Errorf("dst: %w", err)
	}
	resolvedDst, cancelled, err := resolveDstRule(dst, resolvedDst, resolvedSrc, addAdopt)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Println("cancelled")
		return nil
	}
	if ok, err := confirmHomeDst(resolvedDst, home, addForce); !ok || err != nil {
		return err
	}

	if addAdopt {
		if _, err := os.Lstat(resolvedSrc); err == nil {
			return fmt.Errorf("src %s already exists; --adopt moves dst there", resolvedSrc)
		}
		info, err := os.Lstat(resolvedDst)
		if err != nil {
			return fmt.Errorf("dst: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("dst %s is a symlink; nothing to adopt", resolvedDst)
		}
		// A symlinked src parent would redirect the move somewhere the repo owner may not control.
		srcDir := filepath.Dir(resolvedSrc)
		if real, err := filepath.EvalSymlinks(srcDir); err == nil && real != srcDir {
			return fmt.Errorf("src parent %s is a symlink to %s; refusing to move dst through it", srcDir, real)
		}
	} else if _, err := os.Stat(resolvedSrc); err != nil {
		return fmt.Errorf("src does not exist: %w", err)
	}
	entry := config.Entry{Src: resolvedSrc, Dst: resolvedDst}
	if err := config.ValidateEntry(cfg.Entries, entry, -1); err != nil {
		return err
	}
	if !addAdopt {
		cfg.Entries = append(cfg.Entries, entry)
		cfg.Entries = config.NormalizeIDs(cfg.Entries)
		if err := config.Save(cfgPath(), cfg); err != nil {
			return err
		}
		fmt.Printf("added %d: %s -> %s\n", len(cfg.Entries), diff.Escape(resolvedSrc), diff.Escape(resolvedDst))
		if _, err := os.Lstat(resolvedDst); err == nil {
			if target, _ := os.Readlink(resolvedDst); target != resolvedSrc {
				fmt.Fprintf(os.Stderr, "warning: %s already exists; apply will move it to a backup\n", diff.Escape(resolvedDst))
			}
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(resolvedSrc), 0o755); err != nil {
		return err
	}
	// ponytail: rename only, so dst and src must be on the same filesystem (EXDEV otherwise); add copy+remove if repo and home ever live on different mounts.
	if err := os.Rename(resolvedDst, resolvedSrc); err != nil {
		return fmt.Errorf("moving %s to %s: %w", resolvedDst, resolvedSrc, err)
	}
	cfg.Entries = append(cfg.Entries, entry)
	cfg.Entries = config.NormalizeIDs(cfg.Entries)
	if err := config.Save(cfgPath(), cfg); err != nil {
		return fmt.Errorf("%w (file moved to %s; re-run add without --adopt to record it)", err, resolvedSrc)
	}
	if err := os.Symlink(resolvedSrc, resolvedDst); err != nil {
		return fmt.Errorf("linking %s -> %s: %w (entry saved; run dotsync apply)", resolvedDst, resolvedSrc, err)
	}
	fmt.Printf("adopted %s -> %s\n", diff.Escape(resolvedDst), diff.Escape(resolvedSrc))
	return nil
}

// confirmHomeDst asks before using /, $HOME or a parent of $HOME as dst:
// apply would move it aside and --adopt would move it into the repo. It
// returns false (after printing "cancelled") when the user declines.
func confirmHomeDst(dst, home string, force bool) (bool, error) {
	if !config.IsHomeOrAncestor(dst, home) {
		return true, nil
	}
	msg := dst + " is your home directory or one of its parents"
	if force {
		fmt.Fprintln(os.Stderr, "warning: "+diff.Escape(msg))
		return true, nil
	}
	ch, err := prompt.Confirm(msg+"; really use it as dst?", []rune{'y', 'n'}, 'n')
	if err != nil {
		return false, fmt.Errorf("%s; confirm in a terminal or pass --force to add", msg)
	}
	if ch == 'n' {
		fmt.Println("cancelled")
		return false, nil
	}
	return true, nil
}

// resolveDstRule applies the dst rules shared by add and edit: a trailing
// slash links into the directory, and a file over an existing real directory
// prompts for into/replace/cancel. raw is the dst as typed, before cleaning.
func resolveDstRule(raw, resolvedDst, resolvedSrc string, adopt bool) (dst string, cancelled bool, err error) {
	// Like ln -s: a dst ending in / names the directory to link into.
	if strings.HasSuffix(raw, "/") {
		return filepath.Join(resolvedDst, filepath.Base(resolvedSrc)), false, nil
	}
	if adopt {
		return resolvedDst, false, nil
	}
	// A file over an existing real directory, without a trailing slash, is
	// ambiguous: ask whether to link into it or replace it.
	dstInfo, dErr := os.Lstat(resolvedDst)
	srcInfo, sErr := os.Stat(resolvedSrc)
	if dErr == nil && sErr == nil && dstInfo.IsDir() && !srcInfo.IsDir() {
		into := filepath.Join(resolvedDst, filepath.Base(resolvedSrc))
		ch, err := prompt.Confirm(
			fmt.Sprintf("%s is a directory: link [i]nto it as %s, [r]eplace it on apply, or [c]ancel?", resolvedDst, into),
			[]rune{'i', 'r', 'c'}, 'i',
		)
		if err != nil {
			return "", false, fmt.Errorf("%s is a directory: end dst with / to link into it", resolvedDst)
		}
		switch ch {
		case 'i':
			return into, false, nil
		case 'c':
			return "", true, nil
		}
	}
	return resolvedDst, false, nil
}
