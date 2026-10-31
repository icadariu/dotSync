package prompt_test

import (
	"strings"
	"testing"

	"github.com/icadariu/dotSync/internal/prompt"
)

func TestConfirm_ValidChoice(t *testing.T) {
	prompt.Stdin = strings.NewReader("o\n")
	ch, err := prompt.Confirm("test?", []rune{'o', 'c'}, 'c')
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != 'o' {
		t.Errorf("got %c, want 'o'", ch)
	}
}

func TestConfirm_DefaultOnEnter(t *testing.T) {
	prompt.Stdin = strings.NewReader("\n")
	ch, err := prompt.Confirm("test?", []rune{'o', 'c'}, 'c')
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != 'c' {
		t.Errorf("got %c, want 'c'", ch)
	}
}

// TestConfirm_NoInputErrors verifies EOF with nothing typed is an error, not
// the default choice, so a non-interactive run never auto-confirms.
func TestConfirm_NoInputErrors(t *testing.T) {
	for _, in := range []string{"", "z\n"} {
		prompt.Stdin = strings.NewReader(in)
		if ch, err := prompt.Confirm("test?", []rune{'o', 'c'}, 'c'); err == nil {
			t.Errorf("input %q: got %c, want error", in, ch)
		}
	}
}

// TestConfirm_SequentialPromptsShareInput verifies piped answers reach each
// prompt in turn instead of the first prompt buffering all of them.
func TestConfirm_SequentialPromptsShareInput(t *testing.T) {
	prompt.Stdin = strings.NewReader("o\nc\n")
	for _, want := range []rune{'o', 'c'} {
		ch, err := prompt.Confirm("test?", []rune{'o', 'c'}, 'o')
		if err != nil {
			t.Fatalf("want %c: unexpected error: %v", want, err)
		}
		if ch != want {
			t.Errorf("got %c, want %c", ch, want)
		}
	}
}

// TestConfirm_AnswerWithoutNewline verifies a final answer before EOF counts.
func TestConfirm_AnswerWithoutNewline(t *testing.T) {
	prompt.Stdin = strings.NewReader("o")
	ch, err := prompt.Confirm("test?", []rune{'o', 'c'}, 'c')
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != 'o' {
		t.Errorf("got %c, want 'o'", ch)
	}
}

func TestConfirm_InvalidThenValid(t *testing.T) {
	prompt.Stdin = strings.NewReader("z\no\n")
	ch, err := prompt.Confirm("test?", []rune{'o', 'c'}, 'c')
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != 'o' {
		t.Errorf("got %c, want 'o'", ch)
	}
}
