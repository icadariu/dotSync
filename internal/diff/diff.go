package diff

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// IsBinary reports whether data looks like binary (null byte in first 8 KB).
func IsBinary(data []byte) bool {
	return slices.Contains(data[:min(len(data), 8192)], byte(0))
}

// Render returns a unified diff string comparing a (old) to b (new).
// Returns "binary files differ\n" if either side appears binary.
func Render(label string, a, b []byte) string {
	if IsBinary(a) || IsBinary(b) {
		return "binary files differ\n"
	}
	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(a)),
		B:        difflib.SplitLines(string(b)),
		FromFile: "a/" + label,
		ToFile:   "b/" + label,
		Context:  3,
	}
	text, _ := difflib.GetUnifiedDiffString(ud) // bytes.Buffer never errors
	return text
}

// RenderDir returns a unified diff for all files under dstDir vs srcDir.
// dstDir is treated as "old", srcDir as "new" (what apply would install).
// Files present only in src show as additions; only in dst as removals.
// Returns empty string when all files are identical.
func RenderDir(dstDir, srcDir string) string {
	srcPaths := collectRelPaths(srcDir)
	dstPaths := collectRelPaths(dstDir)
	all := unionSorted(srcPaths, dstPaths)

	var sb strings.Builder
	for _, rel := range all {
		srcData, _ := os.ReadFile(filepath.Join(srcDir, rel))
		dstData, _ := os.ReadFile(filepath.Join(dstDir, rel))
		if out := Render(rel, dstData, srcData); out != "" {
			sb.WriteString(out)
		}
	}
	return sb.String()
}

func collectRelPaths(dir string) map[string]bool {
	paths := make(map[string]bool)
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		paths[rel] = true
		return nil
	})
	return paths
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
