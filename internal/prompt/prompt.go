package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// Stdin is the reader used for all prompts. Tests replace this.
var Stdin io.Reader = os.Stdin

// Confirm prompts the user with question and returns the chosen rune.
// options is the list of valid choices; defaultChoice is returned on bare Enter.
// Echoes an [o/i/c]-style hint. Re-prompts on invalid input.
func Confirm(question string, options []rune, defaultChoice rune) (rune, error) {
	hint := buildHint(options, defaultChoice)
	reader := bufio.NewReader(Stdin)
	for {
		fmt.Printf("%s %s: ", question, hint)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
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
