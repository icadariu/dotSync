package linker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/icadariu/dotsync/internal/color"
	"github.com/icadariu/dotsync/internal/config"
	"github.com/icadariu/dotsync/internal/diff"
	"github.com/icadariu/dotsync/internal/prompt"
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

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusLink:
		return "LINK"
	case StatusRelink:
		return "RELINK"
	case StatusConflict:
		return "CONFLICT"
	default:
		return "ERROR"
	}
}

// Result holds the evaluated status for one entry.
type Result struct {
	Entry     config.Entry
	Status    Status
	OldTarget string // non-empty for StatusRelink: current symlink target before relinking
	Message   string // unified diff text for CONFLICT; error reason for ERROR
}

// ErrCancelled is kept for API compatibility but is no longer returned by Apply.
var ErrCancelled = errors.New("cancelled by user")

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

// SafeBackupPath returns dst+suffix if it doesn't exist, otherwise
// dst+suffix+".1", dst+suffix+".2", etc. Exported for testing.
func SafeBackupPath(dst, suffix string) string {
	path := dst + suffix
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return path
	}
	for i := 1; i <= 1000; i++ {
		p := fmt.Sprintf("%s%s.%d", dst, suffix, i)
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
	}
	// Unreachable in practice; return a timestamped fallback rather than panicking.
	return fmt.Sprintf("%s%s.overflow", dst, suffix)
}

func inspectDst(e config.Entry) Result {
	srcInfo, err := os.Stat(e.Src)
	if err != nil {
		return Result{Entry: e, Status: StatusError, Message: fmt.Sprintf("src missing: %v", err)}
	}

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
	var content string
	if srcInfo.IsDir() {
		dstResolved, err := filepath.EvalSymlinks(e.Dst)
		if err != nil {
			dstResolved = e.Dst
		}
		dstInfo, err := os.Stat(dstResolved)
		if err != nil || !dstInfo.IsDir() {
			content = "type mismatch: src is dir, dst is not a dir\n"
		} else {
			content = diff.RenderDir(dstResolved, e.Src)
		}
	} else {
		aData, _ := os.ReadFile(e.Dst) // follows symlink — reads content
		bData, _ := os.ReadFile(e.Src)
		content = diff.Render(filepath.Base(e.Dst), aData, bData)
	}
	header := fmt.Sprintf("old mode %s\nnew mode 120000\n", gitMode(info.Mode()))
	status := StatusConflict
	if info.Mode()&os.ModeSymlink != 0 {
		status = StatusRelink
	}
	return Result{Entry: e, Status: status, OldTarget: oldTarget, Message: header + content}
}

// createMessage builds the "+ create" message: a chezmoi-style header showing
// the new symlink mode, plus a preview of src content for regular files.
func createMessage(e config.Entry, srcInfo os.FileInfo) string {
	header := fmt.Sprintf("new file mode 120000 (symlink -> %s)\n", e.Src)
	if srcInfo.IsDir() {
		return header
	}
	data, err := os.ReadFile(e.Src)
	if err != nil {
		return header
	}
	return header + diff.Render(filepath.Base(e.Dst), nil, data)
}

// Plan returns the evaluated status for every entry without making any changes.
func Plan(entries []config.Entry) []Result {
	results := make([]Result, 0, len(entries))
	for _, e := range entries {
		results = append(results, inspectDst(e))
	}
	return results
}

// Apply applies entries to the filesystem.
// With Force=true, skips per-entry confirmation prompts.
// On conflict, dst is renamed to dst+BackupSuffix unless NoBackup=true, in which case it is deleted.
func Apply(entries []config.Entry, opts ApplyOptions) error {
	for _, e := range entries {
		r := inspectDst(e)

		switch r.Status {
		case StatusError:
			fmt.Fprintf(os.Stderr, "%s %s: %s\n", color.Red("error"), r.Entry.Dst, r.Message)
			continue
		case StatusOK:
			continue
		}

		if !opts.Force {
			if (r.Status == StatusConflict || r.Status == StatusRelink) && r.Message != "" {
				label := "conflict"
				if r.Status == StatusRelink {
					label = "relink"
				}
				fmt.Printf("%s at %s:\n%s", color.Yellow(label), r.Entry.Dst, color.Diff(r.Message))
			}
			ch, err := prompt.Confirm(
				fmt.Sprintf("%s → %s", r.Entry.Dst, r.Entry.Src),
				[]rune{'y', 'n'}, 'y',
			)
			if err != nil {
				return err
			}
			if ch == 'n' {
				continue
			}
		}

		if r.Status == StatusConflict || r.Status == StatusRelink {
			if opts.NoBackup {
				if err := os.Remove(e.Dst); err != nil {
					fmt.Fprintf(os.Stderr, "remove %s: %v\n", e.Dst, err)
					continue
				}
			} else {
				backup := SafeBackupPath(e.Dst, opts.BackupSuffix)
				if err := os.Rename(e.Dst, backup); err != nil {
					fmt.Fprintf(os.Stderr, "backup %s: %v\n", e.Dst, err)
					continue
				}
			}
		}

		if err := os.MkdirAll(filepath.Dir(e.Dst), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", filepath.Dir(e.Dst), err)
			continue
		}
		if err := os.Symlink(e.Src, e.Dst); err != nil {
			fmt.Fprintf(os.Stderr, "symlink %s → %s: %v\n", e.Dst, e.Src, err)
		}
	}
	return nil
}
