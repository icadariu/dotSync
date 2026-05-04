package diff_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/icadariu/dotsync/internal/diff"
)

func TestIsBinary_NullByte(t *testing.T) {
	if !diff.IsBinary([]byte{0x00, 0x01, 0x02}) {
		t.Error("expected binary")
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

func TestRender_Binary(t *testing.T) {
	a := []byte{0x00, 0x01}
	b := []byte("text")
	out := diff.Render("file", a, b)
	if out != "binary files differ\n" {
		t.Errorf("expected binary message, got %q", out)
	}
}

func TestIsBinary_BeyondLimit(t *testing.T) {
	// null byte beyond 8KB should NOT trigger binary detection
	data := make([]byte, 9000)
	// Fill first 8192 bytes with non-zero content
	for i := range 8192 {
		data[i] = 0x01
	}
	data[8500] = 0x00 // beyond 8192 byte limit
	if diff.IsBinary(data) {
		t.Error("null byte beyond 8KB limit should not be detected as binary")
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
