// Managed by github-utils templates from ai-setup (config/claude/user_scripts/go_version_template).
// Edit it there, not in this repo: 'github-utils templates --apply' overwrites local edits.

package main

import (
	"fmt"
	"os"
)

var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

// HandleVersionFlag prints the version line and returns true if --version is
// the FIRST argument; later in argv it belongs to something else (a search
// term, a wrapped command's own flag). Call at the top of main():
//
//	if HandleVersionFlag() { return }
//
// Composes cleanly with cobra, urfave/cli, or plain flag — call it before
// any framework setup so it short-circuits before flag parsing.
func HandleVersionFlag() bool {
	if len(os.Args) < 2 || os.Args[1] != "--version" {
		return false
	}
	fmt.Printf("%s (built %s, commit %s)\n", version, buildTime, commit)
	return true
}
