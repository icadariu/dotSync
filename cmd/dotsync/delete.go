package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/diff"
	"github.com/icadariu/dotSync/internal/prompt"
)

var deleteCmd = &cobra.Command{
	Use:               "delete [<id>]",
	Aliases:           []string{"del"},
	Short:             "Delete a dotfile entry and unlink its symlink",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeEntryIDs,
	RunE:              runDelete,
}

var deleteForce bool

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "skip confirmation prompt")
}

func runDelete(_ *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return err
	}

	var id int
	if len(args) == 0 {
		id, err = pickEntryID(cfg)
		if err != nil {
			return err
		}
	} else {
		id, err = strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid id %q: %w", args[0], err)
		}
		if config.IndexOf(cfg.Entries, id) < 0 {
			if deleteForce {
				return fmt.Errorf("no entry with id %d", id)
			}
			fmt.Printf("no entry with id %d\n\n", id)
			id, err = pickEntryID(cfg)
			if err != nil {
				return err
			}
		}
	}

	idx := config.IndexOf(cfg.Entries, id)
	target := cfg.Entries[idx]

	if !deleteForce {
		ch, err := prompt.Confirm(
			fmt.Sprintf("delete entry %d (%s → %s)?", target.ID, target.Src, target.Dst),
			[]rune{'y', 'n'}, 'n',
		)
		if err != nil {
			return err
		}
		if ch == 'n' {
			fmt.Println("cancelled")
			return nil
		}
	}

	cfg.Entries = append(cfg.Entries[:idx], cfg.Entries[idx+1:]...)
	cfg.Entries = config.NormalizeIDs(cfg.Entries)
	if err := config.Save(cfgPath(), cfg); err != nil {
		return err
	}

	return unlinkIfSymlink(target, id)
}

// ownLink reports whether e.Dst is a symlink pointing exactly at e.Src. When it
// is not, why says so in words (empty when dst does not exist at all).
func ownLink(e config.Entry) (own bool, why string, err error) {
	info, err := os.Lstat(e.Dst)
	if os.IsNotExist(err) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("could not stat %s: %w", e.Dst, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, "is not a symlink", nil
	}
	target, err := os.Readlink(e.Dst)
	if err != nil {
		return false, "", fmt.Errorf("could not read link %s: %w", e.Dst, err)
	}
	if target != e.Src {
		return false, fmt.Sprintf("points to %s, not %s", target, e.Src), nil
	}
	return true, "", nil
}

// unlinkIfSymlink removes e.Dst only when it is a symlink pointing at e.Src;
// real files and links pointing elsewhere are left in place. Failures are
// returned so delete exits non-zero (the entry is already removed by then).
func unlinkIfSymlink(e config.Entry, id int) error {
	own, why, err := ownLink(e)
	if err != nil {
		return fmt.Errorf("removed entry %d, but %w", id, err)
	}
	if !own {
		if why == "" {
			fmt.Printf("removed entry %d (dst not found on disk: %s)\n", id, diff.Escape(e.Dst))
		} else {
			fmt.Printf("removed entry %d; %s %s — left in place\n", id, diff.Escape(e.Dst), diff.Escape(why))
		}
		return nil
	}
	if err := os.Remove(e.Dst); err != nil {
		return fmt.Errorf("removed entry %d, but could not unlink %s: %w", id, e.Dst, err)
	}
	fmt.Printf("removed entry %d and unlinked %s\n", id, diff.Escape(e.Dst))
	return nil
}

func pickEntryID(cfg *config.Config) (int, error) {
	if len(cfg.Entries) == 0 {
		return 0, fmt.Errorf("no entries in config")
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSRC\tDST")
	for _, e := range cfg.Entries {
		fmt.Fprintf(w, "%d\t%s\t%s\n", e.ID, diff.Escape(e.Src), diff.Escape(e.Dst))
	}
	_ = w.Flush()

	for {
		fmt.Print("enter id to delete: ")
		line, err := prompt.ReadLine()
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		id, err := strconv.Atoi(line)
		if err != nil {
			fmt.Printf("invalid id %q, try again\n", line)
			continue
		}
		if config.IndexOf(cfg.Entries, id) >= 0 {
			return id, nil
		}
		fmt.Printf("no entry with id %d, try again\n", id)
	}
}
