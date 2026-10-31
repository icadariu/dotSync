package linker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/icadariu/dotSync/internal/color"
	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/diff"
	"github.com/icadariu/dotSync/internal/prompt"
)

// Status is the outcome of evaluating a single entry.
type Status int

const (
	StatusOK       Status = iota
	StatusLink            // dst absent — create symlink
	StatusRelink          // dst exists with matching content but wrong type
	StatusConflict        // dst exists with diverging content
	StatusError           // src missing or stat failed
)

// Result holds the evaluated status for one entry.
type Result struct {
	Entry     config.Entry
	Status    Status
	OldTarget string // non-empty for StatusRelink: current symlink target before relinking
	Message   string // unified diff text for CONFLICT; error reason for ERROR
}

// ApplyOptions configures Apply behaviour.
type ApplyOptions struct {
	BackupSuffix string
	Force        bool // skip per-entry confirmation prompts
	NoBackup     bool // delete conflicting dst instead of backing it up
}

// gitMode formats a Go FileMode as a git-style 6-digit octal (e.g. 100644 for
// regular files, 120000 for symlinks, 040000 for directories).
func gitMode(m os.FileMode) string {
	switch {
	case m&os.ModeSymlink != 0:
		return "120000"
	case m.IsDir():
		return "040000"
	default:
		return fmt.Sprintf("1%05o", m&0o7777)
	}
}

// ForeignOwned reports whether fi belongs to a uid other than Getuid().
func ForeignOwned(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) != Getuid()
}

// laterDuplicates maps the index of each entry whose dst was already used by an
// earlier entry to that earlier entry's ID; the first entry for a dst wins.
func laterDuplicates(entries []config.Entry) map[int]int {
	first := make(map[string]int, len(entries))
	dups := make(map[int]int)
	for i, e := range entries {
		if id, ok := first[e.Dst]; ok {
			dups[i] = id
			continue
		}
		first[e.Dst] = e.ID
	}
	return dups
}

func inspectDst(e config.Entry, suffix string) Result {
	srcInfo, err := os.Stat(e.Src)
	if err != nil {
		return Result{Entry: e, Status: StatusError, Message: fmt.Sprintf("src missing: %v", err)}
	}
	if err := config.CheckOverlap(e.Src, e.Dst); err != nil {
		return Result{Entry: e, Status: StatusError, Message: err.Error()}
	}

	real, err := filepath.EvalSymlinks(e.Src)
	if err != nil {
		return Result{Entry: e, Status: StatusError, Message: fmt.Sprintf("resolve src: %v", err)}
	}
	for _, p := range []string{real, filepath.Dir(real)} {
		if reason := untrusted(p, p != real); reason != "" {
			return Result{Entry: e, Status: StatusError, Message: "unsafe src: " + reason}
		}
	}
	// A missing dst parent is fine (apply creates it), but the nearest existing
	// ancestor must be trusted, since MkdirAll builds inside it.
	dstParent := filepath.Dir(e.Dst)
	for {
		if _, err := os.Lstat(dstParent); err == nil || filepath.Dir(dstParent) == dstParent {
			break
		}
		dstParent = filepath.Dir(dstParent)
	}
	if resolved, err := filepath.EvalSymlinks(dstParent); err == nil {
		if reason := untrusted(resolved, true); reason != "" {
			return Result{Entry: e, Status: StatusError, Message: "unsafe dst parent: " + reason}
		}
	}
	r := inspectResolved(e, srcInfo, suffix)
	if real != e.Src && (r.Status == StatusLink || r.Status == StatusRelink || r.Status == StatusConflict) {
		r.Message = "src resolves to " + diff.Escape(real) + "\n" + r.Message
	}
	return r
}

// untrusted returns why path may be controlled by another user (owned by
// someone other than us or root, or world-writable), or "" if it is safe. A
// sticky directory is acceptable as a parent, like /tmp. Group-writable is
// allowed (umask 002 setups).
func untrusted(path string, parent bool) string {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("%s: %v", path, err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Uid != 0 && int(st.Uid) != Getuid() {
		return fmt.Sprintf("%s is owned by uid %d", path, st.Uid)
	}
	sticky := parent && info.IsDir() && info.Mode()&os.ModeSticky != 0
	if info.Mode()&0o002 != 0 && !sticky {
		return path + " is world-writable"
	}
	return ""
}

func inspectResolved(e config.Entry, srcInfo os.FileInfo, suffix string) Result {
	info, err := os.Lstat(e.Dst)
	if os.IsNotExist(err) {
		return Result{Entry: e, Status: StatusLink, Message: createMessage(e, srcInfo)}
	}
	if err != nil {
		return Result{Entry: e, Status: StatusError, Message: fmt.Sprintf("stat dst: %v", err)}
	}

	// Correct symlink — nothing to do.
	var oldTarget string
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(e.Dst); err == nil {
			if target == e.Src {
				return Result{Entry: e, Status: StatusOK}
			}
			oldTarget = target
		}
	}

	// dst exists but is wrong. The classification is type-based: when dst is
	// already a symlink (just pointing to the wrong place), this is a relink;
	// any other type — regular file, directory — is a replace.
	// A symlinked dst is never read or walked: its old target is not ours to show.
	var content string
	switch {
	case info.Mode()&os.ModeSymlink != 0:
	case srcInfo.IsDir():
		if info.IsDir() {
			content = diff.RenderDir(e.Dst, e.Src)
		} else {
			content = "type mismatch: src is dir, dst is not a dir\n"
		}
	default:
		content = diff.RenderFile(filepath.Base(e.Dst), e.Dst, e.Src)
	}
	header := fmt.Sprintf("old mode %s\nnew mode 120000\n", gitMode(info.Mode()))
	status := StatusConflict
	if info.Mode()&os.ModeSymlink != 0 {
		status = StatusRelink
	} else if _, err := os.Lstat(e.Dst + suffix); err == nil {
		header += fmt.Sprintf("backup %s will be overwritten\n", diff.Escape(e.Dst+suffix))
	}
	return Result{Entry: e, Status: status, OldTarget: oldTarget, Message: header + content}
}

// createMessage builds the "+ create" message: a chezmoi-style header showing
// the new symlink mode, plus a preview of src content for regular files.
func createMessage(e config.Entry, srcInfo os.FileInfo) string {
	header := fmt.Sprintf("new file mode 120000 (symlink -> %s)\n", diff.Escape(e.Src))
	if srcInfo.IsDir() {
		return header
	}
	return header + diff.RenderFile(filepath.Base(e.Dst), "", e.Src)
}

// Plan returns the evaluated status for every entry without making any changes.
// backupSuffix (default ".bk") is the suffix apply would back up displaced files with.
func Plan(entries []config.Entry, backupSuffix ...string) []Result {
	suffix := ".bk"
	if len(backupSuffix) > 0 {
		suffix = backupSuffix[0]
	}
	results := make([]Result, 0, len(entries))
	dups := laterDuplicates(entries)
	for i, e := range entries {
		if id, ok := dups[i]; ok {
			results = append(results, Result{Entry: e, Status: StatusError, Message: fmt.Sprintf("dst also used by entry %d", id)})
			continue
		}
		results = append(results, inspectDst(e, suffix))
	}
	return results
}

// Apply applies entries to the filesystem.
// With Force=true, skips per-entry confirmation prompts.
// On conflict, dst is renamed to dst+BackupSuffix (replacing an existing backup) unless
// NoBackup=true, in which case it is deleted. A displaced symlink is never backed up.
// Per-entry failures are printed to stderr and processing continues; the returned
// error summarises how many entries failed.
// At the prompt, a answers yes for this and every remaining entry; q stops and
// leaves the remaining entries unchanged.
func Apply(entries []config.Entry, opts ApplyOptions) error {
	failed, linked, unchanged, skipped := 0, 0, 0, 0
	home, _ := os.UserHomeDir() // "" disables the home-dst check
	dups := laterDuplicates(entries)
loop:
	for i, e := range entries {
		if id, ok := dups[i]; ok {
			fmt.Fprintf(os.Stderr, "%s %s: dst also used by entry %d\n", color.Red("error"), diff.Escape(e.Dst), id)
			failed++
			continue
		}
		r := inspectDst(e, opts.BackupSuffix)

		switch r.Status {
		case StatusError:
			fmt.Fprintf(os.Stderr, "%s %s: %s\n", color.Red("error"), diff.Escape(r.Entry.Dst), diff.Escape(r.Message))
			failed++
			continue
		case StatusOK:
			unchanged++
			continue
		}

		backup := e.Dst + opts.BackupSuffix
		var oldBackup os.FileInfo
		if r.Status == StatusConflict && !opts.NoBackup {
			// The old backup is removed (recursively for a dir), so the suffix must
			// name a new sibling: no separator, and never empty (backup == dst).
			if opts.BackupSuffix == "" || strings.ContainsAny(opts.BackupSuffix, "/\x00") {
				fmt.Fprintf(os.Stderr, "%s %s: backup suffix %q leaves the directory of dst\n", color.Red("error"), diff.Escape(e.Dst), opts.BackupSuffix)
				failed++
				continue
			}
			oldBackup, _ = os.Lstat(backup)
			if oldBackup != nil && ForeignOwned(oldBackup) {
				fmt.Fprintf(os.Stderr, "%s %s: backup %s is owned by another user, refusing to overwrite\n", color.Red("error"), diff.Escape(e.Dst), diff.Escape(backup))
				failed++
				continue
			}
		}

		homeDst := home != "" && config.IsHomeOrAncestor(e.Dst, home)
		if homeDst {
			fmt.Fprintf(os.Stderr, "warning: %s is your home directory or one of its parents\n", diff.Escape(e.Dst))
		}
		if !opts.Force {
			if (r.Status == StatusConflict || r.Status == StatusRelink) && r.Message != "" {
				label := "conflict"
				if r.Status == StatusRelink {
					label = "relink"
				}
				fmt.Printf("%s at %s:\n%s", color.Yellow(label), diff.Escape(r.Entry.Dst), color.Diff(r.Message))
			}
			// a bare Enter must not irreversibly delete a real file or an old backup, or move home aside
			def := 'y'
			if homeDst || ((opts.NoBackup || oldBackup != nil) && r.Status == StatusConflict) {
				def = 'n'
			}
			ch, err := prompt.Confirm(
				fmt.Sprintf("%s → %s", diff.Escape(r.Entry.Dst), diff.Escape(r.Entry.Src)),
				[]rune{'y', 'n', 'a', 'q'}, def,
			)
			if err != nil {
				return err
			}
			switch ch {
			case 'n':
				skipped++
				continue
			case 'a':
				opts.Force = true // yes to this and every remaining entry
			case 'q':
				break loop
			}
			// TOCTOU: dst or its backup may have changed while the prompt waited
			r2 := inspectDst(e, opts.BackupSuffix)
			var nb os.FileInfo
			if r.Status == StatusConflict && !opts.NoBackup {
				nb, _ = os.Lstat(backup)
			}
			if r2.Status != r.Status || r2.OldTarget != r.OldTarget || (r.Status == StatusConflict && !opts.NoBackup && (nb == nil) != (oldBackup == nil)) {
				fmt.Fprintf(os.Stderr, "%s %s: changed while waiting for confirmation; re-run apply\n", color.Red("error"), diff.Escape(e.Dst))
				failed++
				continue
			}
		}

		backupNote := ""
		switch {
		case r.Status == StatusRelink:
			// also covers --force, which skips the prompt re-check above
			if fi, err := os.Lstat(e.Dst); err != nil || fi.Mode()&os.ModeSymlink == 0 {
				fmt.Fprintf(os.Stderr, "%s %s: no longer a symlink; re-run apply\n", color.Red("error"), diff.Escape(e.Dst))
				failed++
				continue
			}
			if err := os.Remove(e.Dst); err != nil {
				fmt.Fprintf(os.Stderr, "remove %s: %v\n", diff.Escape(e.Dst), diff.Escape(err.Error()))
				failed++
				continue
			}
			if r.OldTarget != "" {
				backupNote = fmt.Sprintf(" (was -> %s)", diff.Escape(r.OldTarget))
			}
		case r.Status == StatusConflict && opts.NoBackup:
			if fi, err := os.Lstat(e.Dst); err == nil && fi.IsDir() {
				if ents, err := os.ReadDir(e.Dst); err == nil && len(ents) > 0 {
					fmt.Fprintf(os.Stderr, "%s %s: refusing to delete non-empty directory with --no-backup; re-run without it to back it up\n", color.Red("error"), diff.Escape(e.Dst))
					failed++
					continue
				}
			}
			if err := os.Remove(e.Dst); err != nil {
				fmt.Fprintf(os.Stderr, "remove %s: %v\n", diff.Escape(e.Dst), diff.Escape(err.Error()))
				failed++
				continue
			}
			fmt.Fprintf(os.Stderr, "warning: deleted %s (no backup)\n", diff.Escape(e.Dst))
		case r.Status == StatusConflict:
			if oldBackup != nil {
				rm := os.Remove
				if oldBackup.IsDir() {
					rm = os.RemoveAll
				}
				if err := rm(backup); err != nil {
					fmt.Fprintf(os.Stderr, "remove old backup %s: %v\n", diff.Escape(backup), diff.Escape(err.Error()))
					failed++
					continue
				}
				if opts.Force {
					fmt.Fprintf(os.Stderr, "warning: overwrote backup %s\n", diff.Escape(backup))
				}
			}
			if err := os.Rename(e.Dst, backup); err != nil {
				fmt.Fprintf(os.Stderr, "backup %s: %v\n", diff.Escape(e.Dst), diff.Escape(err.Error()))
				failed++
				continue
			}
			backupNote = fmt.Sprintf(" (backup: %s)", diff.Escape(backup))
		}

		if err := os.MkdirAll(filepath.Dir(e.Dst), ancestorPerm(filepath.Dir(e.Dst))); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", diff.Escape(filepath.Dir(e.Dst)), diff.Escape(err.Error()))
			failed++
			continue
		}
		if err := os.Symlink(e.Src, e.Dst); err != nil {
			fmt.Fprintf(os.Stderr, "symlink %s → %s: %v\n", diff.Escape(e.Dst), diff.Escape(e.Src), diff.Escape(err.Error()))
			failed++
			continue
		}
		linked++
		fmt.Printf("linked %s -> %s%s\n", diff.Escape(e.Dst), diff.Escape(e.Src), backupNote)
	}
	summary := fmt.Sprintf("%d linked, %d unchanged", linked, unchanged)
	if failed > 0 {
		summary += fmt.Sprintf(", %d failed", failed)
	}
	if skipped > 0 {
		summary += fmt.Sprintf(", %d skipped", skipped)
	}
	fmt.Println(summary)
	if failed > 0 {
		return fmt.Errorf("%d of %d entries failed", failed, len(entries))
	}
	return nil
}

// ancestorPerm returns the permission bits of dir's nearest existing ancestor
// (plus owner rwx), capped at 0755: directories apply creates are never more
// open than the one they are created in, nor writable by group or others (a
// shared, world-writable ancestor must not spread). umask still applies.
func ancestorPerm(dir string) os.FileMode {
	for p := dir; ; p = filepath.Dir(p) {
		fi, err := os.Stat(p)
		if err == nil {
			return (fi.Mode().Perm() | 0o700) & 0o755
		}
		if !os.IsNotExist(err) || p == filepath.Dir(p) {
			return 0o755
		}
	}
}
