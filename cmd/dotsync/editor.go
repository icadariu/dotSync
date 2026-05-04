package main

import (
	"fmt"
	"os"
	"os/exec"

	"gopkg.in/yaml.v3"

	"github.com/icadariu/dotsync/internal/config"
)

// openEditorForEntry opens $EDITOR (default: vi) on a temp YAML file containing e.
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

	cmd := exec.Command(editor, tmp.Name())
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
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return e, fmt.Errorf("parsing edited entry: %w", err)
	}
	return out, nil
}
