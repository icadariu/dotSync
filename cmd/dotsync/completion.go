package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/icadariu/dotsync/internal/config"
)

// completeEntryIDs returns a ValidArgsFunction that suggests entry IDs from the
// current config, formatted as "<id>\t<src> -> <dst>" so shells can show the
// description next to each completion.
func completeEntryIDs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load(cfgPath())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	out := make([]string, 0, len(cfg.Entries))
	for _, e := range cfg.Entries {
		id := strconv.Itoa(e.ID)
		if !strings.HasPrefix(id, toComplete) {
			continue
		}
		out = append(out, fmt.Sprintf("%s\t%s -> %s", id, e.Src, e.Dst))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
