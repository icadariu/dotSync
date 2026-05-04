// Package color emits git-style ANSI color escapes for diff/status output.
// Disabled when NO_COLOR is set or stdout is not a terminal.
package color

import (
	"os"
	"strings"
)

// Enabled controls whether the helpers emit ANSI escapes. Computed at init
// from NO_COLOR + isatty(stdout); tests may override it.
var Enabled = computeEnabled()

func computeEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

const (
	reset  = "\x1b[0m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
	bold   = "\x1b[1m"
)

// wrapLine applies code to s, placing the reset before any trailing newline so
// the next line starts cleanly. No-op when Enabled is false.
func wrapLine(code, s string) string {
	if !Enabled || s == "" {
		return s
	}
	if strings.HasSuffix(s, "\n") {
		return code + s[:len(s)-1] + reset + "\n"
	}
	return code + s + reset
}

func Red(s string) string    { return wrapLine(red, s) }
func Green(s string) string  { return wrapLine(green, s) }
func Yellow(s string) string { return wrapLine(yellow, s) }
func Cyan(s string) string   { return wrapLine(cyan, s) }
func Bold(s string) string   { return wrapLine(bold, s) }

// Diff colors a unified-diff string git-style: + green, - red, @@ cyan,
// +++/--- bold. Other lines (mode headers, context) pass through unchanged.
func Diff(text string) string {
	if !Enabled || text == "" {
		return text
	}
	var b strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			b.WriteString(Bold(line))
		case strings.HasPrefix(line, "+"):
			b.WriteString(Green(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(Red(line))
		case strings.HasPrefix(line, "@@"):
			b.WriteString(Cyan(line))
		default:
			b.WriteString(line)
		}
	}
	return b.String()
}
