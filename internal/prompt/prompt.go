package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/icadariu/dotSync/internal/diff"
)

// Stdin is the reader used for all prompts. Tests replace this.
var Stdin io.Reader = os.Stdin

// One buffered reader shared by every prompt: a fresh bufio.Reader per prompt
// would swallow the answers meant for later prompts when input is piped.
var (
	buffered     *bufio.Reader
	bufferedFrom io.Reader
)

// ReadLine reads one line from Stdin, including the trailing newline if any.
func ReadLine() (string, error) {
	if buffered == nil || bufferedFrom != Stdin {
		buffered, bufferedFrom = bufio.NewReader(Stdin), Stdin
	}
	return buffered.ReadString('\n')
}

// Confirm prompts the user with question and returns the chosen rune.
// options is the list of valid choices; defaultChoice is returned on bare Enter.
// Echoes an [o/i/c]-style hint. Re-prompts on invalid input.
func Confirm(question string, options []rune, defaultChoice rune) (rune, error) {
	hint := buildHint(options, defaultChoice)
	for {
		fmt.Printf("%s %s: ", diff.Escape(question), hint)
		line, err := ReadLine()
		if err != nil && err != io.EOF {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			// EOF with nothing typed (cron, CI, closed pipe) must never auto-confirm.
			if err == io.EOF {
				fmt.Println()
				return 0, errors.New("no input; run from a terminal or use --force")
			}
			return defaultChoice, nil
		}
		if len(line) == 1 {
			ch := rune(line[0])
			if slices.Contains(options, ch) {
				return ch, nil
			}
		}
		fmt.Printf("invalid choice %q, enter one of %s\n", line, hint)
	}
}

func buildHint(options []rune, defaultChoice rune) string {
	parts := make([]string, len(options))
	for i, o := range options {
		if o == defaultChoice {
			parts[i] = strings.ToUpper(string(o))
		} else {
			parts[i] = string(o)
		}
	}
	return "[" + strings.Join(parts, "/") + "]"
}
