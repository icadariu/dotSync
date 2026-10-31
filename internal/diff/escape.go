package diff

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Escape makes s safe to print to a terminal: C0 controls (except tab) become
// caret notation (ESC -> ^[), DEL becomes ^?, C1 controls become \u00XX and
// invalid UTF-8 bytes become \xXX. Everything else is returned unchanged.
func Escape(s string) string {
	if !strings.ContainsFunc(s, needsEscape) && utf8.ValidString(s) {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&sb, `\x%02x`, s[i])
		case r < 0x20 && r != '\t':
			sb.WriteByte('^')
			sb.WriteRune(r + 0x40)
		case r == 0x7f:
			sb.WriteString("^?")
		case r >= 0x80 && r <= 0x9f, isFormatControl(r):
			fmt.Fprintf(&sb, `\u%04x`, r)
		default:
			sb.WriteString(s[i : i+size])
		}
		i += size
	}
	return sb.String()
}

func needsEscape(r rune) bool {
	return (r < 0x20 && r != '\t') || (r >= 0x7f && r <= 0x9f) || isFormatControl(r)
}

// isFormatControl covers bidi controls and invisible format characters, which
// can make a path display differently from what it is. U+200D ZWJ is left alone so emoji sequences render.
func isFormatControl(r rune) bool {
	switch {
	case r == 0x061C, r == 0x200B, r == 0x200E, r == 0x200F,
		r >= 0x202A && r <= 0x202E,
		r >= 0x2066 && r <= 0x2069,
		r == 0xFEFF:
		return true
	}
	return false
}
