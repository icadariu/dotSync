package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/icadariu/dotsync/internal/config"
	"github.com/icadariu/dotsync/internal/testenv"
)

// testHomeAndRepo creates two subdirs (home, repo) inside a fresh
// /tmp/dotsync_tests/testN directory and returns their paths.
func testHomeAndRepo(t *testing.T) (homeDir, repoDir string) {
	t.Helper()
	dir := testenv.NextTestDir()
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
	dir := testenv.NextTestDir()
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
		if got := loaded.Entries[i]; got != want {
			t.Errorf("entry[%d]: got %+v, want %+v", i, got, want)
		}
	}
}

func TestResolveSrcPath_AbsolutePassesThrough(t *testing.T) {
	got, err := config.ResolveSrcPath("/etc/hosts")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/etc/hosts" {
		t.Errorf("got %q, want %q", got, "/etc/hosts")
	}
}

func TestResolveSrcPath_RelativeResolvesAgainstCWD(t *testing.T) {
	dir := testenv.NextTestDir()
	t.Chdir(dir)

	got, err := config.ResolveSrcPath("sub/file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "sub/file")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSrcPath_TildeExpandsToHome(t *testing.T) {
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	got, err := config.ResolveSrcPath("~/dotfiles/file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(homeDir, "dotfiles/file")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveDstPath_RelativeResolvesToHome(t *testing.T) {
	got := config.ResolveDstPath(".zshrc", "/tmp/home")
	want := "/tmp/home/.zshrc"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveDstPath_TildeExpands(t *testing.T) {
	got := config.ResolveDstPath("~/.config/nvim", "/tmp/home")
	want := "/tmp/home/.config/nvim"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveDstPath_AbsolutePassesThrough(t *testing.T) {
	got := config.ResolveDstPath("/etc/hosts.local", "/tmp/home")
	if got != "/etc/hosts.local" {
		t.Errorf("got %q, want %q", got, "/etc/hosts.local")
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

func TestLoad_DotfilesDirInYAMLSilentlyIgnored(t *testing.T) {
	homeDir, _ := testHomeAndRepo(t)
	t.Setenv("HOME", homeDir)

	// Old configs may still have dotfiles_dir at the top level. yaml.Unmarshal
	// should silently ignore it now that the field is removed.
	cfgPath := filepath.Join(homeDir, ".dotsync.yaml")
	content := "version: 1\ndotfiles_dir: $HOME/dotfiles\nentries:\n  - id: 1\n    src: /etc/hosts\n    dst: /etc/hosts.local\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(cfg.Entries))
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
