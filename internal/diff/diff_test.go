package diff_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/icadariu/dotSync/internal/diff"
)

func TestIsBinary_InvalidUTF8(t *testing.T) {
	if !diff.IsBinary([]byte{0xff, 0x01, 0x02}) {
		t.Error("expected binary")
	}
}

func TestIsBinary_NULAloneIsText(t *testing.T) {
	if diff.IsBinary([]byte{0x00, 0x01, 0x02}) {
		t.Error("NUL bytes alone must not make a file binary")
	}
}

// A rune cut in half by the 8192-byte window is not invalid UTF-8.
func TestIsBinary_RuneTruncatedAtLimit(t *testing.T) {
	data := []byte(strings.Repeat("a", 8191) + "é" + "tail")
	if diff.IsBinary(data) {
		t.Error("rune split by the 8192 cut must not count as invalid")
	}
}

func TestIsBinary_Text(t *testing.T) {
	if diff.IsBinary([]byte("hello\nworld\n")) {
		t.Error("expected not binary")
	}
}

func TestRender_TextDiff(t *testing.T) {
	a := []byte("line1\nline2\n")
	b := []byte("line1\nchanged\n")
	out := diff.Render("file.txt", a, b)
	if !strings.Contains(out, "-line2") {
		t.Errorf("expected removal in diff, got:\n%s", out)
	}
	if !strings.Contains(out, "+changed") {
		t.Errorf("expected addition in diff, got:\n%s", out)
	}
}

// TestRender_NoPhantomTrailingLine matches git: a one-line change is
// "@@ -1 +1 @@" with no invented blank context line.
func TestRender_NoPhantomTrailingLine(t *testing.T) {
	out := diff.Render("x", []byte("precious\n"), []byte("x\n"))
	want := "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-precious\n+x\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestRender_NoTrailingNewline(t *testing.T) {
	out := diff.Render("x", []byte("a\n"), []byte("a\nb"))
	want := "--- a/x\n+++ b/x\n@@ -1 +1,2 @@\n a\n+b\n\\ No newline at end of file\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestRender_Binary(t *testing.T) {
	a := []byte{0xff, 0x01}
	b := []byte("text")
	out := diff.Render("file", a, b)
	if out != "binary files differ\n" {
		t.Errorf("expected binary message, got %q", out)
	}
}

func TestIsBinary_BeyondLimit(t *testing.T) {
	// invalid UTF-8 beyond 8KB should NOT trigger binary detection
	data := make([]byte, 9000)
	// Fill first 8192 bytes with non-zero content
	for i := range 8192 {
		data[i] = 0x01
	}
	data[8500] = 0xff // beyond 8192 byte limit
	if diff.IsBinary(data) {
		t.Error("invalid byte beyond 8KB limit should not be detected as binary")
	}
}

func TestRender_EscapesControlBytes(t *testing.T) {
	out := diff.Render("f", nil, []byte("ok\x1b[2Khidden\n"))
	if !strings.Contains(out, "^[[2K") {
		t.Errorf("expected escaped ESC, got %q", out)
	}
	if strings.Contains(out, "\x1b") {
		t.Errorf("raw ESC leaked: %q", out)
	}
}

func TestRender_EscapesLabel(t *testing.T) {
	out := diff.Render("a\x1b[2Kb", []byte("x\n"), []byte("y\n"))
	if strings.Contains(out, "\x1b") || !strings.Contains(out, "a^[[2Kb") {
		t.Errorf("label not escaped: %q", out)
	}
}

func TestRender_NULInTextStillRendersLines(t *testing.T) {
	out := diff.Render("f", nil, []byte("a\n\x00b\nc\n"))
	if strings.Contains(out, "binary files differ") {
		t.Errorf("NUL must not make the file binary: %q", out)
	}
	if !strings.Contains(out, "+^@b") || !strings.Contains(out, "+c") {
		t.Errorf("expected escaped NUL line and +c, got %q", out)
	}
}

func TestRender_InvalidUTF8IsBinary(t *testing.T) {
	out := diff.Render("f", []byte{0xff, 0xfe, 'a'}, []byte("x\n"))
	if out != "binary files differ\n" {
		t.Errorf("got %q", out)
	}
}

func TestRenderDir_BothEmpty(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	got := diff.RenderDir(dst, src)
	if got != "" {
		t.Errorf("expected empty diff for identical empty dirs, got %q", got)
	}
}

func TestRenderDir_FileOnlyInSrc(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "foo.txt"), []byte("new content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := diff.RenderDir(dst, src)
	if !strings.Contains(got, "+new content") {
		t.Errorf("expected addition line, got:\n%s", got)
	}
}

func TestRenderDir_FileOnlyInDst(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "old.txt"), []byte("old content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := diff.RenderDir(dst, src)
	if !strings.Contains(got, "-old content") {
		t.Errorf("expected removal line, got:\n%s", got)
	}
}

func TestRenderDir_ChangedFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "cfg.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "cfg.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := diff.RenderDir(dst, src)
	if !strings.Contains(got, "-old") || !strings.Contains(got, "+new") {
		t.Errorf("expected change diff, got:\n%s", got)
	}
}

func TestRenderDir_UnchangedFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	content := []byte("same content\n")
	if err := os.WriteFile(filepath.Join(src, "same.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "same.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	got := diff.RenderDir(dst, src)
	if got != "" {
		t.Errorf("expected empty diff for unchanged files, got:\n%s", got)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A huge real dst (e.g. /usr) must not be walked without bound: entries
// deeper than MaxDiffDepth are skipped and a note says so.
func TestRenderDir_StopsBelowMaxDepth(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(dst, "a/b/c/at-limit.txt"), "in-limit\n")
	writeTestFile(t, filepath.Join(dst, "a/b/c/d/too-deep.txt"), "too-deep\n")
	got := diff.RenderDir(dst, src)
	if !strings.Contains(got, "-in-limit") {
		t.Errorf("file at depth %d should be diffed, got:\n%s", diff.MaxDiffDepth, got)
	}
	if strings.Contains(got, "too-deep") {
		t.Errorf("file below depth %d should not be diffed, got:\n%s", diff.MaxDiffDepth, got)
	}
	if !strings.Contains(got, "not diffed") {
		t.Errorf("expected a truncation note, got:\n%s", got)
	}
}

// Depth alone does not bound a shallow, wide dir, so the file count is capped too.
func TestRenderDir_StopsAfterMaxFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	for i := 0; i <= diff.MaxDiffFiles; i++ {
		writeTestFile(t, filepath.Join(dst, fmt.Sprintf("f%05d", i)), "x\n")
	}
	got := diff.RenderDir(dst, src)
	if n := strings.Count("\n"+got, "\n--- "); n > diff.MaxDiffFiles {
		t.Errorf("diffed %d files, want at most %d", n, diff.MaxDiffFiles)
	}
	if !strings.Contains(got, "not diffed") {
		t.Errorf("expected a truncation note at the end, got tail:\n%s", got[max(0, len(got)-300):])
	}
}

// TestRenderDir_FIFOInDst_DoesNotBlock verifies RenderDir does not open a
// FIFO inside dst.
func TestRenderDir_FIFOInDst_DoesNotBlock(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dst, "a"), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		diff.RenderDir(dst, src)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RenderDir blocked on a FIFO in dst")
	}
}

// ---- RenderDir never reads through symlinks (#91) ----

func symlinkFixture(t *testing.T) (secret, srcDir, dstDir string) {
	t.Helper()
	root := t.TempDir()
	secret = filepath.Join(root, "outside", "secret.txt")
	srcDir = filepath.Join(root, "src")
	dstDir = filepath.Join(root, "dst")
	for _, d := range []string{filepath.Dir(secret), srcDir, dstDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(secret, []byte("FAKE-SECRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return secret, srcDir, dstDir
}

func TestRenderDir_SymlinkShownAsTarget(t *testing.T) {
	secret, srcDir, dstDir := symlinkFixture(t)
	if err := os.Symlink(secret, filepath.Join(srcDir, "theme")); err != nil {
		t.Fatal(err)
	}
	out := diff.RenderDir(dstDir, srcDir)
	if !strings.Contains(out, secret) {
		t.Errorf("expected symlink target %q in output, got:\n%s", secret, out)
	}
	if strings.Contains(out, "FAKE-SECRET") {
		t.Errorf("RenderDir read through symlink:\n%s", out)
	}
}

func TestRenderDir_SymlinkInDstNotFollowed(t *testing.T) {
	secret, srcDir, dstDir := symlinkFixture(t)
	if err := os.Symlink(secret, filepath.Join(dstDir, "theme")); err != nil {
		t.Fatal(err)
	}
	out := diff.RenderDir(dstDir, srcDir)
	if strings.Contains(out, "FAKE-SECRET") {
		t.Errorf("RenderDir read through dst symlink:\n%s", out)
	}
}
