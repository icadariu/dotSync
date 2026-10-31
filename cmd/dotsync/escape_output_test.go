package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/icadariu/dotSync/internal/color"
	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/prompt"
	"github.com/icadariu/dotSync/internal/testenv"
)

// escEntryEnv writes a config whose dst name carries an ESC sequence, as a
// crafted shared config could, and turns color off so any \x1b is a leak.
func escEntryEnv(t *testing.T) *testenv.Env {
	t.Helper()
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/rc": "x\n"})
	env.WriteConfig([]config.Entry{{ID: 1, Src: filepath.Join(env.RepoDir, "files/rc"), Dst: filepath.Join(env.HomeDir, "d\x1b[2Kst")}})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	old := color.Enabled
	color.Enabled = false
	t.Cleanup(func() { color.Enabled = old })
	return env
}

func assertEscaped(t *testing.T, what, out string) {
	t.Helper()
	if strings.Contains(out, "\x1b") {
		t.Errorf("%s leaked a raw ESC byte: %q", what, out)
	}
	if !strings.Contains(out, "^[") {
		t.Errorf("%s should show the escaped path, got %q", what, out)
	}
}

func TestE2E_List_EscapesPaths(t *testing.T) {
	escEntryEnv(t)
	rootCmd.SetArgs([]string{"list"})
	out := captureStdout(func() { _ = rootCmd.Execute() })
	assertEscaped(t, "list", out)
}

func TestE2E_Delete_EscapesPromptAndOutput(t *testing.T) {
	escEntryEnv(t)
	prompt.Stdin = strings.NewReader("y\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	rootCmd.SetArgs([]string{"delete", "1"})
	out := captureStdout(func() { _ = rootCmd.Execute() })
	assertEscaped(t, "delete", out)
}

func TestE2E_Unlink_EscapesPaths(t *testing.T) {
	escEntryEnv(t)
	rootCmd.SetArgs([]string{"unlink", "--force"})
	out := captureStdout(func() { _ = rootCmd.Execute() })
	assertEscaped(t, "unlink", out)
}

func TestErrorLine_Escapes(t *testing.T) {
	if got := errorLine(errors.New("bad \x1b[2K path")); got != "bad ^[[2K path" {
		t.Errorf("errorLine = %q, want the ESC escaped", got)
	}
}

func TestE2E_Plan_EscapesResolvedSrc(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"real\x1b[2J/rc": "x\n"})
	link := filepath.Join(env.RepoDir, "link")
	if err := os.Symlink(filepath.Join(env.RepoDir, "real\x1b[2J/rc"), link); err != nil {
		t.Fatal(err)
	}
	env.WriteConfig([]config.Entry{{ID: 1, Src: link, Dst: filepath.Join(env.HomeDir, "dst")}})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	old := color.Enabled
	color.Enabled = false
	t.Cleanup(func() { color.Enabled = old })

	rootCmd.SetArgs([]string{"plan", "--no-pager"})
	out := captureStdout(func() { _ = rootCmd.Execute() })
	if !strings.Contains(out, "src resolves to") {
		t.Fatalf("expected the 'src resolves to' line, got %q", out)
	}
	assertEscaped(t, "plan", out)
}

func TestE2E_Unlink_EscapesErrors(t *testing.T) {
	env := escEntryEnv(t)
	dst := filepath.Join(env.HomeDir, "p\x1b[2Kdir", "d\x1b[2Kst")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(env.RepoDir, "files/rc")
	if err := os.Symlink(src, dst); err != nil {
		t.Fatal(err)
	}
	env.WriteConfig([]config.Entry{{ID: 1, Src: src, Dst: dst}})
	// A read-only parent makes os.Remove fail with "could not unlink <dst>".
	if err := os.Chmod(filepath.Dir(dst), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(dst), 0o755) })

	rootCmd.SetArgs([]string{"unlink", "--force"})
	var out string
	errOut := captureStderr(func() {
		out = captureStdout(func() { _ = rootCmd.Execute() })
	})
	_ = out
	if !strings.Contains(errOut, "could not unlink") {
		t.Fatalf("expected an unlink error on stderr, got %q", errOut)
	}
	assertEscaped(t, "unlink stderr", errOut)
}

func TestEntryIDCompletions_EscapesDescriptions(t *testing.T) {
	escEntryEnv(t)
	got, _ := entryIDCompletions(nil, "")
	if len(got) != 1 {
		t.Fatalf("want 1 completion, got %q", got)
	}
	assertEscaped(t, "completion", got[0])
}
