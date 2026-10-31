package diff

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"
)

// MaxDiffSize is the largest file, in bytes, that is read and diffed.
const MaxDiffSize = 1 << 20

// MaxDiffDepth and MaxDiffFiles bound the walk of each side in RenderDir, so
// plan stays fast when dst is a huge real directory (like /usr).
const (
	MaxDiffDepth = 4
	MaxDiffFiles = 1000
)

// IsBinary reports whether data looks like binary (invalid UTF-8 in the first
// 8 KB). A rune cut in half by the 8 KB limit does not count as invalid.
func IsBinary(data []byte) bool {
	head := data[:min(len(data), 8192)]
	if len(head) < len(data) {
		// drop a rune the cut truncated
		for i := len(head) - 1; i >= 0 && i >= len(head)-4; i-- {
			if utf8.RuneStart(head[i]) {
				if !utf8.FullRune(head[i:]) {
					head = head[:i]
				}
				break
			}
		}
	}
	return !utf8.Valid(head)
}

// Render returns a unified diff string comparing a (old) to b (new).
// Returns "binary files differ\n" if either side appears binary.
func Render(label string, a, b []byte) string {
	if IsBinary(a) || IsBinary(b) {
		return "binary files differ\n"
	}
	// a newline in the label would survive the per-line escaping below
	label = strings.ReplaceAll(label, "\n", "^J")
	ud := difflib.UnifiedDiff{
		A:        splitLines(a),
		B:        splitLines(b),
		FromFile: "a/" + label,
		ToFile:   "b/" + label,
		Context:  3,
	}
	text, _ := difflib.GetUnifiedDiffString(ud) // bytes.Buffer never errors
	// Escape after diffing so a control byte and its escaped spelling never
	// compare equal. Raw lines hold no "\n", so each piece is a whole output line.
	pieces := strings.Split(text, "\n")
	for i, p := range pieces {
		pieces[i] = Escape(p)
	}
	return strings.Join(pieces, "\n")
}

// splitLines splits data into newline-terminated lines. Unlike
// difflib.SplitLines it adds no phantom empty line after a trailing newline,
// and it marks a missing final newline the way git does.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	if last := lines[len(lines)-1]; last == "" {
		lines = lines[:len(lines)-1]
	} else {
		lines[len(lines)-1] = last + "\n\\ No newline at end of file\n"
	}
	return lines
}

// RenderDir returns a unified diff for all files under dstDir vs srcDir.
// dstDir is treated as "old", srcDir as "new" (what apply would install).
// Files present only in src show as additions; only in dst as removals.
// Returns empty string when all files are identical.
func RenderDir(dstDir, srcDir string) string {
	srcPaths, srcCut := collectRelPaths(srcDir)
	dstPaths, dstCut := collectRelPaths(dstDir)
	all := unionSorted(srcPaths, dstPaths)

	var sb strings.Builder
	for _, rel := range all {
		sb.WriteString(RenderFile(rel, filepath.Join(dstDir, rel), filepath.Join(srcDir, rel)))
	}
	if srcCut || dstCut {
		fmt.Fprintf(&sb, "... further entries not diffed (limits: depth %d, %d files)\n", MaxDiffDepth, MaxDiffFiles)
	}
	return sb.String()
}

// collectRelPaths returns the files under dir down to MaxDiffDepth levels, at
// most MaxDiffFiles of them, and whether anything was left out.
func collectRelPaths(dir string) (map[string]bool, bool) {
	paths := make(map[string]bool)
	cut := false
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			// a dir at depth MaxDiffDepth-1 holds files at MaxDiffDepth; deeper is skipped
			if rel != "." && strings.Count(rel, string(filepath.Separator))+1 >= MaxDiffDepth {
				cut = true
				return filepath.SkipDir
			}
			return nil
		}
		if len(paths) == MaxDiffFiles {
			cut = true
			return filepath.SkipAll
		}
		paths[rel] = true
		return nil
	})
	return paths, cut
}

func unionSorted(a, b map[string]bool) []string {
	merged := make(map[string]bool, len(a)+len(b))
	for k := range a {
		merged[k] = true
	}
	for k := range b {
		merged[k] = true
	}
	out := make([]string, 0, len(merged))
	for k := range merged {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// readSide returns what a diff shows for path and its size in bytes. It never
// follows symlinks: a symlink yields its target text, like git. Regular files
// larger than MaxDiffSize yield nil data (only the size is reported), and
// anything else (missing, FIFO, device) yields nil, 0 so a diff never blocks.
func readSide(path string) (data []byte, size int64) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return nil, 0
		}
		return []byte(target), int64(len(target))
	case info.Mode().IsRegular():
		if info.Size() > MaxDiffSize {
			return nil, info.Size()
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, 0
		}
		defer f.Close()
		// +1 so a file that grew after Lstat is still detected as too large.
		data, _ = io.ReadAll(io.LimitReader(f, MaxDiffSize+1))
		if len(data) > MaxDiffSize {
			return nil, int64(len(data))
		}
		return data, int64(len(data))
	}
	return nil, 0
}

// RenderFile returns a unified diff of the file at dstPath (old) vs srcPath
// (new), without following symlinks. If either side exceeds MaxDiffSize it
// returns "file too large to diff (N bytes)\n" with N the larger size.
func RenderFile(label, dstPath, srcPath string) string {
	a, aSize := readSide(dstPath)
	b, bSize := readSide(srcPath)
	if n := max(aSize, bSize); n > MaxDiffSize {
		return fmt.Sprintf("file too large to diff (%d bytes)\n", n)
	}
	return Render(label, a, b)
}
