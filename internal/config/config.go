package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Entry struct {
	ID  int    `yaml:"id"`
	Src string `yaml:"src"`
	Dst string `yaml:"dst"`
}

type Config struct {
	Version      int     `yaml:"version"`
	BackupSuffix string  `yaml:"backup_suffix"`
	Entries      []Entry `yaml:"entries"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, ".dotsync.yaml"), nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Version: 1, BackupSuffix: ".bk"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := resolvePaths(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// resolvePaths expands $HOME / ~/ in stored entries and rejects anything that
// isn't absolute after expansion. Paths are required to be absolute on disk
// since the dotfiles_dir indirection was removed.
func resolvePaths(cfg *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home directory: %w", err)
	}

	for i, e := range cfg.Entries {
		src := expandHome(e.Src, home)
		if !filepath.IsAbs(src) {
			return fmt.Errorf("entry %d has non-absolute src %q; re-run `dotsync add` to update entries to absolute paths", e.ID, e.Src)
		}
		cfg.Entries[i].Src = src

		dst := expandHome(e.Dst, home)
		if !filepath.IsAbs(dst) {
			return fmt.Errorf("entry %d has non-absolute dst %q; re-run `dotsync add` to update entries to absolute paths", e.ID, e.Dst)
		}
		cfg.Entries[i].Dst = dst
	}

	return nil
}

func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// NormalizeIDs reassigns IDs 1..N in slice order, eliminating gaps.
func NormalizeIDs(entries []Entry) []Entry {
	for i := range entries {
		entries[i].ID = i + 1
	}
	return entries
}

// SortEntriesBySrc sorts entries by Src ascending (case-sensitive, stable).
// It does not modify IDs; call NormalizeIDs afterwards to renumber.
func SortEntriesBySrc(entries []Entry) []Entry {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Src < entries[j].Src
	})
	return entries
}

// expandHome replaces $HOME and ~/ with the real home directory.
func expandHome(path, home string) string {
	switch {
	case strings.HasPrefix(path, "$HOME/"):
		return filepath.Join(home, path[len("$HOME/"):])
	case path == "$HOME":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	case path == "~":
		return home
	}
	return path
}

// ResolveSrcPath turns CLI src input into an absolute path.
// Absolute inputs are cleaned and returned. ~/ expands against $HOME.
// Relative inputs resolve against the current working directory.
func ResolveSrcPath(input string) (string, error) {
	if strings.HasPrefix(input, "~/") || input == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding home directory: %w", err)
		}
		input = expandHome(input, home)
	}
	if filepath.IsAbs(input) {
		return filepath.Clean(input), nil
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", input, err)
	}
	return abs, nil
}

// ResolveDstPath resolves a dst input against the home directory.
// Absolute inputs pass through unchanged. ~/... and bare relative paths
// are joined with home.
func ResolveDstPath(input, home string) string {
	if filepath.IsAbs(input) {
		return filepath.Clean(input)
	}
	if strings.HasPrefix(input, "~/") {
		return filepath.Join(home, input[2:])
	}
	return filepath.Join(home, input)
}
