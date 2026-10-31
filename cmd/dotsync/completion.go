package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/diff"
)

// completeEntryIDs returns a ValidArgsFunction that suggests entry IDs from the
// current config, formatted as "<id>\t<src> -> <dst>" so shells can show the
// description next to each completion.
func completeEntryIDs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return entryIDCompletions(nil, toComplete)
}

// completeEntryIDsMulti completes any number of entry IDs (plan, apply),
// skipping IDs already given.
func completeEntryIDsMulti(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return entryIDCompletions(args, toComplete)
}

// entryIDCompletions returns "<id>\t<src> -> <dst>" for every entry whose ID
// starts with toComplete and is not listed in exclude.
func entryIDCompletions(exclude []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	out := make([]string, 0, len(cfg.Entries))
	for _, e := range cfg.Entries {
		id := strconv.Itoa(e.ID)
		if slices.Contains(exclude, id) {
			continue
		}
		if !strings.HasPrefix(id, toComplete) {
			continue
		}
		out = append(out, fmt.Sprintf("%s\t%s -> %s", id, diff.Escape(e.Src), diff.Escape(e.Dst)))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
