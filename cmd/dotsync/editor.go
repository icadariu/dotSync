package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"gopkg.in/yaml.v3"

	"github.com/icadariu/dotSync/internal/config"
)

// openEditorForEntry opens $EDITOR (default: vi; may include arguments) on a temp YAML file containing e.
// Returns the parsed entry from the saved file.
func openEditorForEntry(e config.Entry) (config.Entry, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	data, err := yaml.Marshal(e)
	if err != nil {
		return e, err
	}
	tmp, err := os.CreateTemp("", "dotsync-entry-*.yaml")
	if err != nil {
		return e, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		return e, err
	}
	tmp.Close()

	// Run via sh so $EDITOR may carry arguments (e.g. "code --wait"), as git does.
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return e, fmt.Errorf("editor: %w", err)
	}
	raw, err := os.ReadFile(tmp.Name())
	if err != nil {
		return e, err
	}
	var out config.Entry
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&out); err != nil && !errors.Is(err, io.EOF) {
		return e, fmt.Errorf("parsing edited entry: %w", err)
	}
	return out, nil
}
