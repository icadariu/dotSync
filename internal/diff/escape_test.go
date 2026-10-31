package diff_test

import (
	"testing"

	"github.com/icadariu/dotSync/internal/diff"
)

func TestEscape(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"esc sequence", "\x1b[2K", "^[[2K"},
		{"cr", "a\rb", "a^Mb"},
		{"nul", "\x00", "^@"},
		{"del", "\x7f", "^?"},
		{"lf", "a\nb", "a^Jb"},
		{"tab unchanged", "a\tb", "a\tb"},
		{"c1 csi", "\u009b", "\\u009b"},
		{"invalid byte", "a\xffb", "a\\xffb"},
		{"unicode unchanged", "é", "é"},
		{"zwj unchanged", "\u200d", "\u200d"},
		{"u+061c", "\u061c", "\\u061c"},
		{"u+200b", "\u200b", "\\u200b"},
		{"u+200e", "\u200e", "\\u200e"},
		{"u+200f", "\u200f", "\\u200f"},
		{"u+202a", "\u202a", "\\u202a"},
		{"u+202b", "\u202b", "\\u202b"},
		{"u+202c", "\u202c", "\\u202c"},
		{"u+202d", "\u202d", "\\u202d"},
		{"u+202e", "\u202e", "\\u202e"},
		{"u+2066", "\u2066", "\\u2066"},
		{"u+2067", "\u2067", "\\u2067"},
		{"u+2068", "\u2068", "\\u2068"},
		{"u+2069", "\u2069", "\\u2069"},
		{"u+feff", "\ufeff", "\\ufeff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := diff.Escape(tt.in); got != tt.want {
				t.Errorf("Escape(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Escaping happens after diffing, so a real ESC byte and the literal text
// "^[" never compare equal and hide a change.
func TestRender_LiteralCaretVsControlByteDiffers(t *testing.T) {
	if out := diff.Render("f", []byte("^[x\n"), []byte("\x1bx\n")); out == "" {
		t.Fatal("a literal ^[ line and an ESC line must render as a change")
	}
}
