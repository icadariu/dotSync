package color_test

import (
	"strings"
	"testing"

	"github.com/icadariu/dotsync/internal/color"
)

func withEnabled(t *testing.T, on bool) {
	t.Helper()
	prev := color.Enabled
	color.Enabled = on
	t.Cleanup(func() { color.Enabled = prev })
}

func TestDiff_DisabledIsIdentity(t *testing.T) {
	withEnabled(t, false)
	in := "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n"
	if got := color.Diff(in); got != in {
		t.Errorf("disabled should pass through unchanged, got %q", got)
	}
}

func TestDiff_ColorsAdditionsRemovalsAndHunks(t *testing.T) {
	withEnabled(t, true)
	in := "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n context\n"
	out := color.Diff(in)
	for _, want := range []string{
		"\x1b[1m--- a/x\x1b[0m\n", // header bold
		"\x1b[1m+++ b/x\x1b[0m\n",
		"\x1b[36m@@ -1 +1 @@\x1b[0m\n", // hunk cyan
		"\x1b[31m-old\x1b[0m\n",        // removal red
		"\x1b[32m+new\x1b[0m\n",        // addition green
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected substring %q in output:\n%q", want, out)
		}
	}
	if !strings.Contains(out, " context\n") {
		t.Errorf("context line should be unchanged, got:\n%q", out)
	}
}

func TestWrappers_DisabledIsIdentity(t *testing.T) {
	withEnabled(t, false)
	for _, fn := range []func(string) string{color.Red, color.Green, color.Yellow, color.Cyan, color.Bold} {
		if got := fn("hello"); got != "hello" {
			t.Errorf("expected pass-through, got %q", got)
		}
	}
}

func TestRed_EnabledWrapsWithReset(t *testing.T) {
	withEnabled(t, true)
	if got := color.Red("oops"); got != "\x1b[31moops\x1b[0m" {
		t.Errorf("got %q", got)
	}
}

func TestWrap_ResetBeforeNewline(t *testing.T) {
	withEnabled(t, true)
	if got := color.Green("+x\n"); got != "\x1b[32m+x\x1b[0m\n" {
		t.Errorf("reset must precede the trailing newline, got %q", got)
	}
}
