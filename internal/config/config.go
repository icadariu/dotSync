package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// Load reads the config at path; a missing file is an error wrapping fs.ErrNotExist.
// Entry IDs are renumbered 1..N in file order.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("config %s: %w (run dotsync add to create it)", path, fs.ErrNotExist)
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		if strings.Contains(err.Error(), "field dotfiles_dir not found") {
			return nil, fmt.Errorf("parsing config: dotfiles_dir was removed; entries now store absolute src/dst paths, delete the key: %w", err)
		}
		if strings.Contains(err.Error(), "field hosts not found") {
			return nil, fmt.Errorf("parsing config: hosts was removed; per-host entries are no longer supported, delete the key (keep one config per machine instead): %w", err)
		}
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Version != 0 && cfg.Version != 1 {
		return nil, fmt.Errorf("config %s: unsupported version %d (want 1)", path, cfg.Version)
	}
	if cfg.BackupSuffix == "" {
		cfg.BackupSuffix = ".bk"
	}
	// apply renames dst to dst+suffix, so a separator would move it elsewhere.
	if strings.ContainsAny(cfg.BackupSuffix, "/\x00") || cfg.BackupSuffix == "." || cfg.BackupSuffix == ".." {
		return nil, fmt.Errorf("config %s: invalid backup_suffix %q (must not contain / or be . or ..)", path, cfg.BackupSuffix)
	}
	if err := resolvePaths(&cfg); err != nil {
		return nil, err
	}
	cfg.Entries = NormalizeIDs(cfg.Entries)
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
		cfg.Entries[i].Src = filepath.Clean(src)

		dst := expandHome(e.Dst, home)
		if !filepath.IsAbs(dst) {
			return fmt.Errorf("entry %d has non-absolute dst %q; re-run `dotsync add` to update entries to absolute paths", e.ID, e.Dst)
		}
		// Cleaned so home/./.bashrc and home/.bashrc count as one dst.
		cfg.Entries[i].Dst = filepath.Clean(dst)
	}

	return nil
}

func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	// Follow a symlinked config (e.g. ~/.dotsync.yaml linked from a dotfiles
	// repo) so the rename below replaces the target, not the link.
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("resolving config symlink %s: %w", path, err)
		}
		path = resolved
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	// Write a temp file and rename it into place so a crash or full disk
	// mid-write can never leave a truncated config.
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op once the rename succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
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

// IndexOf returns the index of the entry with the given ID, or -1.
func IndexOf(entries []Entry, id int) int {
	return slices.IndexFunc(entries, func(e Entry) bool { return e.ID == id })
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

// ResolvePath resolves a src or dst input to an absolute path. ~ and $HOME (alone
// or as a ~/ or $HOME/ prefix) expand to home; absolute inputs are cleaned;
// any other relative input resolves against the current working directory.
// An empty input is an error rather than the cwd.
func ResolvePath(input, home string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty path")
	}
	input = expandHome(input, home)
	if filepath.IsAbs(input) {
		return filepath.Clean(input), nil
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", input, err)
	}
	return abs, nil
}

// CheckOverlap rejects a src/dst pair that would make apply replace src with a
// symlink to itself (or, with --no-backup, delete it): identical paths, one
// path nested inside the other, or a dst that is not a symlink but is the same
// file as src (e.g. reached through a symlinked parent directory).
func CheckOverlap(src, dst string) error {
	src, dst = filepath.Clean(src), filepath.Clean(dst)
	switch {
	case src == dst:
		return fmt.Errorf("src and dst are the same path: %s", src)
	case isNested(src, dst):
		return fmt.Errorf("dst %s is inside src %s", dst, src)
	case isNested(dst, src):
		return fmt.Errorf("src %s is inside dst %s", src, dst)
	}
	dstInfo, err := os.Lstat(dst)
	if err != nil || dstInfo.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		return nil
	}
	if os.SameFile(srcInfo, dstInfo) {
		return fmt.Errorf("dst %s is the same file as src %s", dst, src)
	}
	return nil
}

// IsHomeOrAncestor reports whether dst is /, home, or a directory home is
// nested in: using one as dst would move the whole home directory aside.
func IsHomeOrAncestor(dst, home string) bool {
	dst, home = filepath.Clean(dst), filepath.Clean(home)
	return dst == "/" || dst == home || isNested(dst, home)
}

// isNested reports whether child is strictly inside parent.
func isNested(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ValidateEntry checks e against the other entries (skip is the index of the
// entry being replaced by edit, or -1 for a new entry) and against CheckOverlap.
func ValidateEntry(entries []Entry, e Entry, skip int) error {
	for i, other := range entries {
		if i != skip && other.Dst == e.Dst {
			return fmt.Errorf("entry %d already uses dst %s", other.ID, e.Dst)
		}
	}
	return CheckOverlap(e.Src, e.Dst)
}
