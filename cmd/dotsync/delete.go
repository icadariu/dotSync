package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/config"
	"github.com/icadariu/dotsync/internal/prompt"
)

var deleteCmd = &cobra.Command{
	Use:               "delete [<id>]",
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
		found := false
		for _, e := range cfg.Entries {
			if e.ID == id {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("no entry with id %d\n\n", id)
			id, err = pickEntryID(cfg)
			if err != nil {
				return err
			}
		}
	}

	idx := -1
	var target config.Entry
	for i, e := range cfg.Entries {
		if e.ID == id {
			idx = i
			target = e
			break
		}
	}

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

	return unlinkIfSymlink(target.Dst, id)
}

func unlinkIfSymlink(dst string, id int) error {
	info, err := os.Lstat(dst)
	if os.IsNotExist(err) {
		fmt.Printf("removed entry %d (dst not found on disk: %s)\n", id, dst)
		return nil
	}
	if err != nil {
		fmt.Printf("removed entry %d; could not stat dst: %v\n", id, err)
		return nil
	}
	if info.Mode()&os.ModeSymlink == 0 {
		fmt.Printf("removed entry %d; %s is not a symlink — left in place\n", id, dst)
		return nil
	}
	if err := os.Remove(dst); err != nil {
		fmt.Printf("removed entry %d; could not unlink %s: %v\n", id, dst, err)
		return nil
	}
	fmt.Printf("removed entry %d and unlinked %s\n", id, dst)
	return nil
}

func pickEntryID(cfg *config.Config) (int, error) {
	if len(cfg.Entries) == 0 {
		return 0, fmt.Errorf("no entries in config")
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSRC\tDST")
	for _, e := range cfg.Entries {
		fmt.Fprintf(w, "%d\t%s\t%s\n", e.ID, e.Src, e.Dst)
	}
	_ = w.Flush()

	reader := bufio.NewReader(prompt.Stdin)
	for {
		fmt.Print("enter id to delete: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		id, err := strconv.Atoi(line)
		if err != nil {
			fmt.Printf("invalid id %q, try again\n", line)
			continue
		}
		for _, e := range cfg.Entries {
			if e.ID == id {
				return id, nil
			}
		}
		fmt.Printf("no entry with id %d, try again\n", id)
	}
}
