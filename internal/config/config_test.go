package config_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/icadariu/dotSync/internal/config"
)

// testHomeAndRepo creates two subdirs (home, repo) inside a fresh
// per-test temp directory and returns their paths.
func testHomeAndRepo(t *testing.T) (homeDir, repoDir string) {
	t.Helper()
	dir := t.TempDir()
	homeDir = filepath.Join(dir, "home")
	repoDir = filepath.Join(dir, "repo")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll home: %v", err)
	}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	return
}

func TestConfig_NormalizeIDs(t *testing.T) {
	tests := []struct {
		ids  []int
		want []int
	}{
		{[]int{1, 3, 5}, []int{1, 2, 3}},
		{[]int{1, 2, 3}, []int{1, 2, 3}},
		{[]int{}, []int{}},
		{[]int{5}, []int{1}},
	}
	for _, tt := range tests {
		entries := make([]config.Entry, len(tt.ids))
		for i, id := range tt.ids {
			entries[i] = config.Entry{ID: id}
		}
		got := config.NormalizeIDs(entries)
		for i, e := range got {
			if e.ID != tt.want[i] {
				t.Errorf("NormalizeIDs(%v)[%d] = %d, want %d", tt.ids, i, e.ID, tt.want[i])
			}
		}
	}
}

func TestConfig_IndexOf(t *testing.T) {
	entries := []config.Entry{{ID: 1}, {ID: 2}, {ID: 3}}
	for id, want := range map[int]int{1: 0, 3: 2, 4: -1} {
		if got := config.IndexOf(entries, id); got != want {
			t.Errorf("IndexOf(%d) = %d, want %d", id, got, want)
		}
	}
}

func TestConfig_SortEntriesBySrc(t *testing.T) {
	t.Run("already sorted is unchanged", func(t *testing.T) {
		entries := []config.Entry{
			{ID: 1, Src: "/a"},
			{ID: 2, Src: "/b"},
			{ID: 3, Src: "/c"},
		}
		got := config.SortEntriesBySrc(entries)
		for i, want := range []string{"/a", "/b", "/c"} {
			if got[i].Src != want {
				t.Errorf("got[%d].Src = %q, want %q", i, got[i].Src, want)
			}
		}
	})

	t.Run("reverse becomes ascending", func(t *testing.T) {
		entries := []config.Entry{
			{ID: 1, Src: "/c"},
			{ID: 2, Src: "/b"},
			{ID: 3, Src: "/a"},
		}
		got := config.SortEntriesBySrc(entries)
		for i, want := range []string{"/a", "/b", "/c"} {
			if got[i].Src != want {
				t.Errorf("got[%d].Src = %q, want %q", i, got[i].Src, want)
			}
		}
	})

	t.Run("empty and single do not panic", func(t *testing.T) {
		_ = config.SortEntriesBySrc(nil)
		_ = config.SortEntriesBySrc([]config.Entry{})
		got := config.SortEntriesBySrc([]config.Entry{{ID: 7, Src: "/x"}})
		if len(got) != 1 || got[0].Src != "/x" {
			t.Errorf("single-entry sort altered slice: %+v", got)
		}
	})

	t.Run("case-sensitive (uppercase before lowercase)", func(t *testing.T) {
		entries := []config.Entry{
			{ID: 1, Src: "/home/a"},
			{ID: 2, Src: "/home/A"},
		}
		got := config.SortEntriesBySrc(entries)
		if got[0].Src != "/home/A" || got[1].Src != "/home/a" {
			t.Errorf("case-sensitive order broken: %+v", got)
		}
	})

	t.Run("stable when src is equal", func(t *testing.T) {
		// Two entries with identical Src — original relative order must hold.
		entries := []config.Entry{
			{ID: 10, Src: "/same", Dst: "/dst-first"},
			{ID: 20, Src: "/same", Dst: "/dst-second"},
		}
		got := config.SortEntriesBySrc(entries)
		if got[0].Dst != "/dst-first" || got[1].Dst != "/dst-second" {
			t.Errorf("stability broken: %+v", got)
		}
	})
}

func TestConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".dotsync.yaml")

	original := &config.Config{
		Version:      1,
		BackupSuffix: ".bk",
		Entries: []config.Entry{
			{ID: 1, Src: "/repo/files/.zshrc", Dst: "/home/user/.zshrc"},
			{ID: 2, Src: "/repo/dirs/kitty", Dst: "/home/user/.config/kitty"},
		},
	}

	if err := config.Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Entries) != len(original.Entries) {
		t.Fatalf("got %d entries, want %d", len(loaded.Entries), len(original.Entries))
	}
	for i, want := range original.Entries {
		if got := loaded.Entries[i]; !reflect.DeepEqual(got, want) {
			t.Errorf("entry[%d]: got %+v, want %+v", i, got, want)
		}
	}
}

func TestResolvePath_SubdirResolvesAgainstCWD(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	got, err := config.ResolvePath("sub/file", "/tmp/home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "sub/file")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolvePath_TildeExpandsToHome_FromEnvHome(t *testing.T) {
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	got, err := config.ResolvePath("~/dotfiles/file", homeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(homeDir, "dotfiles/file")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolvePath_RelativeResolvesToCwd(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	got, err := config.ResolvePath(".zshrc", "/tmp/home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, ".zshrc")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolvePath_TildeExpands(t *testing.T) {
	got, err := config.ResolvePath("~/.config/nvim", "/tmp/home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/tmp/home/.config/nvim"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolvePath_AbsolutePassesThrough(t *testing.T) {
	got, err := config.ResolvePath("/etc/hosts.local", "/tmp/home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/etc/hosts.local" {
		t.Errorf("got %q, want %q", got, "/etc/hosts.local")
	}
}

func TestResolvePath_Forms(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	cwd := filepath.Join(dir, "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	tests := []struct {
		input, want string
	}{
		{"~", home},
		{"$HOME", home},
		{"$HOME/x", filepath.Join(home, "x")},
		{"~/x", filepath.Join(home, "x")},
		{"./x", filepath.Join(cwd, "x")},
		{"../x", filepath.Join(dir, "x")},
		{"x", filepath.Join(cwd, "x")},
		{"a/b", filepath.Join(cwd, "a/b")},
		{"/abs/x", "/abs/x"},
	}
	for _, tc := range tests {
		got, err := config.ResolvePath(tc.input, home)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestResolvePath_EmptyRejected verifies an empty src or dst (e.g. a line
// cleared in edit's $EDITOR) errors instead of silently becoming the cwd.
func TestResolvePath_EmptyRejected(t *testing.T) {
	if got, err := config.ResolvePath("", "/tmp/home"); err == nil {
		t.Errorf("ResolvePath(\"\") = %q, want error", got)
	}
}

func TestCheckOverlap(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(real, "f")
	if err := os.WriteFile(f, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(f, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		src, dst string
		wantErr  bool
	}{
		{"same path", "/s", "/s", true},
		{"dst inside src", "/s", "/s/x", true},
		{"src inside dst", "/s/x", "/s", true},
		{"sibling prefix", "/a/b", "/a/bc", false},
		{"normal", "/repo/.zshrc", "/home/.zshrc", false},
		{"same file via symlinked parent", f, filepath.Join(alias, "f"), true},
		{"dst is symlink to src", f, link, false},
	}
	for _, tc := range tests {
		err := config.CheckOverlap(tc.src, tc.dst)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestLoad_AbsoluteSrcAndDstUnchanged(t *testing.T) {
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	content := "version: 1\nentries:\n  - id: 1\n    src: /etc/hosts\n    dst: /etc/hosts.local\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Entries[0].Src != "/etc/hosts" {
		t.Errorf("Src = %q, want /etc/hosts", cfg.Entries[0].Src)
	}
	if cfg.Entries[0].Dst != "/etc/hosts.local" {
		t.Errorf("Dst = %q, want /etc/hosts.local", cfg.Entries[0].Dst)
	}
}

func TestLoad_DollarHOMEExpandsInEntries(t *testing.T) {
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	content := "version: 1\nentries:\n  - id: 1\n    src: $HOME/dotfiles/.zshrc\n    dst: $HOME/.zshrc\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	wantSrc := filepath.Join(homeDir, "dotfiles/.zshrc")
	wantDst := filepath.Join(homeDir, ".zshrc")
	if cfg.Entries[0].Src != wantSrc {
		t.Errorf("Src = %q, want %q", cfg.Entries[0].Src, wantSrc)
	}
	if cfg.Entries[0].Dst != wantDst {
		t.Errorf("Dst = %q, want %q", cfg.Entries[0].Dst, wantDst)
	}
}

func TestLoad_RelativeSrcErrorsWithGuidance(t *testing.T) {
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	content := "version: 1\nentries:\n  - id: 1\n    src: linux/.zshrc\n    dst: /tmp/.zshrc\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for non-absolute src, got nil")
	}
	if !strings.Contains(err.Error(), "non-absolute src") {
		t.Errorf("error message = %q, want it to mention non-absolute src", err.Error())
	}
}

// writeRawConfig writes content to a fresh config file under a sandbox HOME
// and returns its path.
func writeRawConfig(t *testing.T, content string) string {
	t.Helper()
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)
	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return cfgPath
}

func TestLoad_DotfilesDirLegacyHint(t *testing.T) {
	cfgPath := writeRawConfig(t, "version: 1\ndotfiles_dir: $HOME/dotfiles\nentries:\n  - id: 1\n    src: /etc/hosts\n    dst: /etc/hosts.local\n")

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("Load: want error for legacy dotfiles_dir, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"dotfiles_dir", "removed", "absolute"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

func TestLoad_HostsKeyRemovedHint(t *testing.T) {
	cfgPath := writeRawConfig(t, "version: 1\nentries:\n  - id: 1\n    src: /etc/hosts\n    dst: /etc/hosts.local\n    hosts: [laptop]\n")

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("Load: want error for removed hosts key, got nil")
	}
	if !strings.Contains(err.Error(), "hosts was removed") {
		t.Errorf("error %q does not contain %q", err.Error(), "hosts was removed")
	}
}

func TestLoad_UnknownKeyErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		key     string
	}{
		{"top-level typo", "version: 1\nbackup_sufix: .old\nentries: []\n", "backup_sufix"},
		{"entry-level unknown", "version: 1\nentries:\n  - id: 1\n    src: /etc/hosts\n    dest: /etc/hosts.local\n", "dest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath := writeRawConfig(t, tt.content)
			_, err := config.Load(cfgPath)
			if err == nil {
				t.Fatalf("Load: want error for unknown key %q, got nil", tt.key)
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.key) {
				t.Errorf("error %q does not name key %q", msg, tt.key)
			}
			if !strings.Contains(msg, "line") {
				t.Errorf("error %q does not mention a line number", msg)
			}
		})
	}
}

func TestLoad_VersionMismatchErrors(t *testing.T) {
	cfgPath := writeRawConfig(t, "version: 7\nentries: []\n")

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("Load: want error for version 7, got nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error %q does not mention version", err)
	}
}

// backup_suffix is appended to dst for a rename, so a separator or dot name
// would let the config move dst anywhere.
func TestLoad_UnsafeBackupSuffixErrors(t *testing.T) {
	for _, suffix := range []string{"/../../x", "a/b", ".", "..", "\"x\\0y\""} {
		cfgPath := writeRawConfig(t, "version: 1\nbackup_suffix: "+suffix+"\nentries: []\n")
		if _, err := config.Load(cfgPath); err == nil || !strings.Contains(err.Error(), "backup_suffix") {
			t.Errorf("suffix %s: want backup_suffix error, got %v", suffix, err)
		}
	}
}

// Stored paths are cleaned so home/./.bashrc and home/.bashrc are one dst.
func TestLoad_CleansPaths(t *testing.T) {
	cfgPath := writeRawConfig(t, "entries:\n  - src: /repo/./a/\n    dst: /home/u/./.a\n")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e := cfg.Entries[0]; e.Src != "/repo/a" || e.Dst != "/home/u/.a" {
		t.Errorf("got src %q dst %q, want cleaned /repo/a and /home/u/.a", e.Src, e.Dst)
	}
}

func TestLoad_MissingVersionOK(t *testing.T) {
	cfgPath := writeRawConfig(t, "entries:\n  - id: 1\n    src: /etc/hosts\n    dst: /etc/hosts.local\n")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(cfg.Entries))
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	cfgPath := writeRawConfig(t, "")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(cfg.Entries))
	}
}

func TestSave_PathsRemainAbsolute(t *testing.T) {
	homeDir, repoDir := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	src := filepath.Join(repoDir, "linux/.zshrc")
	dst := filepath.Join(homeDir, ".zshrc")
	cfg := &config.Config{
		Version:      1,
		BackupSuffix: ".bk",
		Entries: []config.Entry{
			{ID: 1, Src: src, Dst: dst},
		},
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	raw := string(data)

	if !strings.Contains(raw, "src: "+src) {
		t.Errorf("expected absolute src %q in YAML, got:\n%s", src, raw)
	}
	if !strings.Contains(raw, "dst: "+dst) {
		t.Errorf("expected absolute dst %q in YAML, got:\n%s", dst, raw)
	}
	if strings.Contains(raw, "$HOME") {
		t.Errorf("expected no $HOME contraction in YAML, got:\n%s", raw)
	}
	if strings.Contains(raw, "dotfiles_dir") {
		t.Errorf("expected no dotfiles_dir key in YAML, got:\n%s", raw)
	}
}

func TestSave_AbsolutePathsOutsidePrefixUnchanged(t *testing.T) {
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	cfg := &config.Config{
		Version: 1,
		Entries: []config.Entry{
			{ID: 1, Src: "/etc/hosts", Dst: "/etc/hosts.local"},
		},
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	raw := string(data)

	if !strings.Contains(raw, "src: /etc/hosts") {
		t.Errorf("expected absolute src unchanged in YAML, got:\n%s", raw)
	}
	if !strings.Contains(raw, "dst: /etc/hosts.local") {
		t.Errorf("expected absolute dst unchanged in YAML, got:\n%s", raw)
	}
}

// TestSave_ReplacesFileAtomically checks the config is written to a temp file
// and renamed into place (a new inode), so a crash mid-write cannot truncate
// it, and that no temp file is left behind.
func TestSave_ReplacesFileAtomically(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".dotsync.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := config.Save(cfgPath, &config.Config{Version: 1}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	after, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("Save rewrote the config in place; want write-to-temp + rename")
	}
	if names, _ := os.ReadDir(dir); len(names) != 1 {
		t.Errorf("expected only the config in %s, found %d entries", dir, len(names))
	}
}

// TestSave_SymlinkedConfigKeepsLink checks a symlinked config (e.g. linked
// from a dotfiles repo) stays a symlink, and its target gets the new content
// with its mode preserved.
func TestSave_SymlinkedConfigKeepsLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "repo", "dotsync.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".dotsync.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Version: 1, Entries: []config.Entry{{ID: 1, Src: "/etc/hosts", Dst: "/etc/hosts.local"}}}
	if err := config.Save(link, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config symlink was replaced (err %v)", err)
	}
	data, _ := os.ReadFile(target)
	if !strings.Contains(string(data), "src: /etc/hosts") {
		t.Errorf("target not updated, got:\n%s", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("target mode = %v, want 0600 preserved", info.Mode().Perm())
	}
}

// TestSave_BrokenSymlinkErrors checks Save refuses to replace a dangling
// config symlink with a regular file.
func TestSave_BrokenSymlinkErrors(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, ".dotsync.yaml")
	if err := os.Symlink(filepath.Join(dir, "missing", "dotsync.yaml"), link); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(link, &config.Config{Version: 1}); err == nil {
		t.Fatal("expected error for a broken config symlink")
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("broken config symlink was replaced (err %v)", err)
	}
}

func TestRoundTrip_AbsolutePathsSurvive(t *testing.T) {
	homeDir, repoDir := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	src := filepath.Join(repoDir, "linux/.zshrc")
	dst := filepath.Join(homeDir, ".zshrc")
	initial := "version: 1\nbackup_suffix: .bk\nentries:\n  - id: 1\n    src: " + src + "\n    dst: " + dst + "\n"
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg1, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := config.Save(cfgPath, cfg1); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfg2, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load (second): %v", err)
	}

	if cfg1.Entries[0].Src != cfg2.Entries[0].Src {
		t.Errorf("Src changed: %q → %q", cfg1.Entries[0].Src, cfg2.Entries[0].Src)
	}
	if cfg1.Entries[0].Dst != cfg2.Entries[0].Dst {
		t.Errorf("Dst changed: %q → %q", cfg1.Entries[0].Dst, cfg2.Entries[0].Dst)
	}
}

// TestLoad_MissingFile_ErrorsNotExist checks a missing config is an error
// wrapping fs.ErrNotExist that names the path and hints at `dotsync add`.
func TestLoad_MissingFile_ErrorsNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.yaml")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error does not wrap fs.ErrNotExist: %v", err)
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "dotsync add") {
		t.Errorf("error should contain path and %q, got: %v", "dotsync add", err)
	}
}

// TestLoad_MissingBackupSuffix_DefaultsToBk checks Load fills in ".bk" when
// backup_suffix is absent.
func TestLoad_MissingBackupSuffix_DefaultsToBk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nentries: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BackupSuffix != ".bk" {
		t.Errorf("BackupSuffix = %q, want %q", cfg.BackupSuffix, ".bk")
	}
}

// TestLoad_RenumbersDuplicateAndMissingIDs: a hand-edited file with duplicate
// or absent ids must come back as 1..N in file order.
func TestLoad_RenumbersDuplicateAndMissingIDs(t *testing.T) {
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	content := "version: 1\nentries:\n" +
		"  - id: 3\n    src: /r/a\n    dst: /h/a\n" +
		"  - id: 3\n    src: /r/b\n    dst: /h/b\n" +
		"  - src: /r/c\n    dst: /h/c\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	wantSrcs := []string{"/r/a", "/r/b", "/r/c"}
	if len(cfg.Entries) != len(wantSrcs) {
		t.Fatalf("got %d entries, want %d", len(cfg.Entries), len(wantSrcs))
	}
	for i, want := range wantSrcs {
		if cfg.Entries[i].Src != want {
			t.Errorf("Entries[%d].Src = %q, want %q", i, cfg.Entries[i].Src, want)
		}
		if cfg.Entries[i].ID != i+1 {
			t.Errorf("Entries[%d].ID = %d, want %d", i, cfg.Entries[i].ID, i+1)
		}
	}
}
