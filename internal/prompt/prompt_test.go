package prompt_test

import (
	"strings"
	"testing"

	"github.com/icadariu/dotsync/internal/prompt"
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
