package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/icadariu/dotsync/internal/config"
	"github.com/icadariu/dotsync/internal/prompt"
	"github.com/icadariu/dotsync/internal/testenv"
)

// captureStdout runs f and returns everything written to os.Stdout.
func captureStdout(f func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var sb strings.Builder
	_, _ = io.Copy(&sb, r)
	return sb.String()
}

// TestE2E_AddAndList exercises add + list.
func TestE2E_AddAndList(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")

	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"list"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, ".zshrc") {
		t.Errorf("list output missing .zshrc:\n%s", out)
	}
	if strings.Contains(out, "OS") || strings.Contains(out, "TYPE") {
		t.Errorf("list output must not contain OS or TYPE columns:\n%s", out)
	}
}

// TestE2E_ListAlias verifies the ls alias works.
func TestE2E_ListAlias(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"ls"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, ".zshrc") {
		t.Errorf("ls alias output missing .zshrc:\n%s", out)
	}
}

// TestE2E_AddDuplicateDst returns an error.
func TestE2E_AddDuplicateDst(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n", "files/.zshrc2": "# alt\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", filepath.Join(env.RepoDir, "files/.zshrc"), "--dest", dst})
	_ = rootCmd.Execute()

	rootCmd.SetArgs([]string{"add", "--src", filepath.Join(env.RepoDir, "files/.zshrc2"), "--dest", dst})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error on duplicate dst, got nil")
	}
}

// TestE2E_Plan_ShowsCreate verifies plan output before apply.
func TestE2E_Plan_ShowsCreate(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "+ create") {
		t.Errorf("plan missing '+ create':\n%s", out)
	}
}

// TestE2E_ApplyForce creates symlinks without prompts.
func TestE2E_ApplyForce(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	rootCmd.SetArgs([]string{"apply", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	env.AssertSymlink(t, ".zshrc", src)
}

// TestE2E_Plan_ShowsUnchangedAfterApply verifies plan shows = unchanged after apply.
func TestE2E_Plan_ShowsUnchangedAfterApply(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()
	rootCmd.SetArgs([]string{"apply", "--force"})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan", "--verbose"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "= unchanged") {
		t.Errorf("plan --verbose should show = unchanged after apply:\n%s", out)
	}
}

// TestE2E_Apply_ConflictBacksUpRealFile verifies backup on real-file conflict.
func TestE2E_Apply_ConflictBacksUpRealFile(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	rootCmd.SetArgs([]string{"apply", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	env.AssertSymlink(t, ".zshrc", src)
	env.AssertBackup(t, ".zshrc", "# old\n")
}

// TestE2E_Plan_ShowsDiff verifies plan shows unified diff on conflict.
func TestE2E_Plan_ShowsDiff(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	var buf bytes.Buffer
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	_ = buf
	if !strings.Contains(out, "~ replace") {
		t.Errorf("plan should show ~ replace:\n%s", out)
	}
	if !strings.Contains(out, "-# old") || !strings.Contains(out, "+# new") {
		t.Errorf("plan should contain unified diff:\n%s", out)
	}
}

// TestE2E_Delete_UnlinksSymlink verifies delete removes the symlink from disk.
func TestE2E_Delete_UnlinksSymlink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()
	rootCmd.SetArgs([]string{"apply", "--force"})
	_ = rootCmd.Execute()
	env.AssertSymlink(t, ".zshrc", src)

	rootCmd.SetArgs([]string{"delete", "1", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	env.AssertNotExist(t, ".zshrc")

	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 0 {
		t.Errorf("expected 0 entries after delete, got %d", len(cfg.Entries))
	}
}

// TestE2E_Delete_LeavesRealFileInPlace verifies real files are not deleted.
func TestE2E_Delete_LeavesRealFileInPlace(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	env.WriteHome(map[string]string{".zshrc": "# real file\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	rootCmd.SetArgs([]string{"delete", "1", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	env.AssertRegular(t, ".zshrc", "# real file\n")
}

// TestE2E_Delete_MissingID_ListsAndAsks verifies the fallback prompt when id not found.
func TestE2E_Delete_MissingID_ListsAndAsks(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	prompt.Stdin = strings.NewReader("1\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	rootCmd.SetArgs([]string{"delete", "99", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete with missing id: %v", err)
	}
	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 0 {
		t.Errorf("expected entry to be deleted, got %d entries", len(cfg.Entries))
	}
}

// TestE2E_Plan_ReplaceOnRegularFileWithIdenticalContent verifies that a
// regular file at dst — even with content identical to src — is classified
// as "~ replace", not "~ relink". The mode header is emitted but no diff
// hunks since content matches.
func TestE2E_Plan_ReplaceOnRegularFileWithIdenticalContent(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# same\n"})
	env.WriteHome(map[string]string{".zshrc": "# same\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "~ replace") {
		t.Errorf("plan should show ~ replace for regular file at dst, got:\n%s", out)
	}
	if strings.Contains(out, "~ relink") {
		t.Errorf("plan must not show ~ relink when dst is a regular file:\n%s", out)
	}
	if !strings.Contains(out, "old mode 1006") || !strings.Contains(out, "new mode 120000") {
		t.Errorf("plan should print old/new mode header, got:\n%s", out)
	}
	if strings.Contains(out, "@@ ") {
		t.Errorf("plan must not include diff hunks for identical content:\n%s", out)
	}
}

// TestE2E_Plan_RelinkOnWrongSymlink verifies that a symlink at dst pointing
// to the wrong target produces "~ relink" — the file type does not change,
// only the target.
func TestE2E_Plan_RelinkOnWrongSymlink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{
		"files/.zshrc": "# new\n",
		"old/.zshrc":   "# old target\n",
	})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	wrongTarget := filepath.Join(env.RepoDir, "old/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	if err := os.Symlink(wrongTarget, dst); err != nil {
		t.Fatalf("setup symlink: %v", err)
	}

	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "~ relink") {
		t.Errorf("plan should show ~ relink for wrong-target symlink, got:\n%s", out)
	}
	if strings.Contains(out, "~ replace") {
		t.Errorf("plan must not show ~ replace when dst is already a symlink:\n%s", out)
	}
	wantTargetLine := fmt.Sprintf("    %s → %s", wrongTarget, src)
	if !strings.Contains(out, wantTargetLine) {
		t.Errorf("plan should show old→new target line %q, got:\n%s", wantTargetLine, out)
	}
}

// TestE2E_Apply_ReplaceBacksUpRegularFileWithIdenticalContent verifies apply
// still backs up and re-symlinks even when content is identical.
func TestE2E_Apply_ReplaceBacksUpRegularFileWithIdenticalContent(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# same\n"})
	env.WriteHome(map[string]string{".zshrc": "# same\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	rootCmd.SetArgs([]string{"apply", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	env.AssertSymlink(t, ".zshrc", src)
	env.AssertBackup(t, ".zshrc", "# same\n")
}

// TestE2E_Plan_ShowsCreateContent verifies plan emits the new symlink mode
// and a content preview when dst does not exist yet.
func TestE2E_Plan_ShowsCreateContent(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\nalias ll='ls -la'\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "new file mode 120000") {
		t.Errorf("plan should report new symlink mode, got:\n%s", out)
	}
	if !strings.Contains(out, "+# zsh") || !strings.Contains(out, "+alias ll") {
		t.Errorf("plan should preview src content as additions, got:\n%s", out)
	}
}

// TestE2E_Plan_ShowsModeTransitionOnReplace verifies plan prints both old and
// new mode when an existing regular file would be replaced by a symlink.
func TestE2E_Plan_ShowsModeTransitionOnReplace(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "old mode 1006") {
		t.Errorf("plan should report old regular-file mode (1006xx), got:\n%s", out)
	}
	if !strings.Contains(out, "new mode 120000") {
		t.Errorf("plan should report new symlink mode, got:\n%s", out)
	}
}

// TestE2E_Completion_DeleteSuggestsEntryIDs drives cobra's hidden __complete
// command and asserts that delete/edit return existing entry IDs.
func TestE2E_Completion_DeleteSuggestsEntryIDs(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{
		"files/.zshrc": "# zsh\n",
		"files/.vimrc": "\" vim\n",
	})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	for _, pair := range [][2]string{
		{"files/.zshrc", ".zshrc"},
		{"files/.vimrc", ".vimrc"},
	} {
		rootCmd.SetArgs([]string{"add",
			"--src", filepath.Join(env.RepoDir, pair[0]),
			"--dest", filepath.Join(env.HomeDir, pair[1]),
		})
		_ = rootCmd.Execute()
	}

	for _, sub := range []string{"delete", "edit"} {
		out := captureStdout(func() {
			rootCmd.SetArgs([]string{"__complete", sub, ""})
			_ = rootCmd.Execute()
		})
		if !strings.Contains(out, "1\t") || !strings.Contains(out, "2\t") {
			t.Errorf("%s completion missing ids 1/2:\n%s", sub, out)
		}
		if !strings.Contains(out, ".zshrc") || !strings.Contains(out, ".vimrc") {
			t.Errorf("%s completion missing src descriptions:\n%s", sub, out)
		}
	}
}

// TestE2E_Completion_PrefixFilters verifies the toComplete prefix filters IDs.
func TestE2E_Completion_PrefixFilters(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{
		"files/a": "a\n",
		"files/b": "b\n",
	})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	for _, pair := range [][2]string{{"files/a", "a"}, {"files/b", "b"}} {
		rootCmd.SetArgs([]string{"add",
			"--src", filepath.Join(env.RepoDir, pair[0]),
			"--dest", filepath.Join(env.HomeDir, pair[1]),
		})
		_ = rootCmd.Execute()
	}

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"__complete", "delete", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "1\t") {
		t.Errorf("expected id 1 in completion output:\n%s", out)
	}
	// id 2 must not appear when prefix is "1"
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "2\t") {
			t.Errorf("id 2 should not appear when prefix is '1':\n%s", out)
		}
	}
}

// TestE2E_IDNormalize verifies that IDs remain sequential (1, 2, 3…) after
// delete+add cycles — no gaps, no reuse of old positions.
func TestE2E_IDNormalize(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{
		"files/.zshrc":  "# zsh\n",
		"files/.vimrc":  "\" vim\n",
		"files/.bashrc": "# bash\n",
	})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	for _, pair := range [][2]string{
		{"files/.zshrc", ".zshrc"},
		{"files/.vimrc", ".vimrc"},
		{"files/.bashrc", ".bashrc"},
	} {
		rootCmd.SetArgs([]string{"add",
			"--src", filepath.Join(env.RepoDir, pair[0]),
			"--dest", filepath.Join(env.HomeDir, pair[1]),
		})
		_ = rootCmd.Execute()
	}

	// Delete the middle entry; IDs should compact to 1(.zshrc), 2(.bashrc).
	rootCmd.SetArgs([]string{"delete", "2", "--force"})
	_ = rootCmd.Execute()

	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 2 {
		t.Fatalf("expected 2 entries after delete, got %d", len(cfg.Entries))
	}
	for i, e := range cfg.Entries {
		if e.ID != i+1 {
			t.Errorf("entry[%d].ID = %d, want %d", i, e.ID, i+1)
		}
	}

	// Add a new entry; it should receive ID 3.
	env.WriteRepo(map[string]string{"files/.tmux.conf": "# tmux\n"})
	rootCmd.SetArgs([]string{"add",
		"--src", filepath.Join(env.RepoDir, "files/.tmux.conf"),
		"--dest", filepath.Join(env.HomeDir, ".tmux.conf"),
	})
	_ = rootCmd.Execute()

	cfg, _ = config.Load(env.ConfigPath)
	if len(cfg.Entries) != 3 {
		t.Fatalf("expected 3 entries after second add, got %d", len(cfg.Entries))
	}
	for i, e := range cfg.Entries {
		if e.ID != i+1 {
			t.Errorf("entry[%d].ID = %d, want %d", i, e.ID, i+1)
		}
	}
}

// TestE2E_Add_RelativeSrc_ResolvedAgainstCWD verifies a relative --src is
// resolved against the current working directory, not against any dotfiles_dir.
func TestE2E_Add_RelativeSrc_ResolvedAgainstCWD(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"linux/.zshrc": "# zsh\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	t.Chdir(env.RepoDir)

	rootCmd.SetArgs([]string{"add",
		"--src", "linux/.zshrc",
		"--dest", ".zshrc",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cfg.Entries))
	}
	wantSrc := filepath.Join(env.RepoDir, "linux/.zshrc")
	wantDst := filepath.Join(env.HomeDir, ".zshrc")
	if cfg.Entries[0].Src != wantSrc {
		t.Errorf("Src = %q, want %q", cfg.Entries[0].Src, wantSrc)
	}
	if cfg.Entries[0].Dst != wantDst {
		t.Errorf("Dst = %q, want %q", cfg.Entries[0].Dst, wantDst)
	}
}

// TestE2E_Add_AbsoluteSrcStoredAbsolute verifies that an absolute --src is
// stored as-is in the YAML, with no contraction or relative encoding.
func TestE2E_Add_AbsoluteSrcStoredAbsolute(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"shared/.gitconfig": "[user]\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	fullSrc := filepath.Join(env.RepoDir, "shared/.gitconfig")
	rootCmd.SetArgs([]string{"add",
		"--src", fullSrc,
		"--dest", ".gitconfig",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	data, _ := os.ReadFile(env.ConfigPath)
	raw := string(data)
	if !strings.Contains(raw, "src: "+fullSrc) {
		t.Errorf("expected absolute src %q in YAML, got:\n%s", fullSrc, raw)
	}
	if strings.Contains(raw, "dotfiles_dir") {
		t.Errorf("config must not contain dotfiles_dir key, got:\n%s", raw)
	}
}

// TestE2E_Add_NonexistentSrc_Errors verifies add rejects a src that doesn't
// exist on disk and writes no entry.
func TestE2E_Add_NonexistentSrc_Errors(t *testing.T) {
	env := testenv.New(t)
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	missing := filepath.Join(env.RepoDir, "does/not/exist")
	rootCmd.SetArgs([]string{"add",
		"--src", missing,
		"--dest", ".zshrc",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent src, got nil")
	}
	if !strings.Contains(err.Error(), "src does not exist") {
		t.Errorf("error = %q, want it to mention 'src does not exist'", err.Error())
	}

	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 0 {
		t.Errorf("expected no entries, got %d", len(cfg.Entries))
	}
}

// TestE2E_Add_RelativeDst_ResolvedToHome verifies that a bare relative dest
// (e.g. ".zshrc") is resolved against $HOME, not cwd.
func TestE2E_Add_RelativeDst_ResolvedToHome(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"linux/.zshrc": "# zsh\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "linux/.zshrc")
	rootCmd.SetArgs([]string{"add",
		"--src", src,
		"--dest", ".zshrc",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	cfg, _ := config.Load(env.ConfigPath)
	want := filepath.Join(env.HomeDir, ".zshrc")
	if cfg.Entries[0].Dst != want {
		t.Errorf("Dst = %q, want %q", cfg.Entries[0].Dst, want)
	}
}

// TestE2E_Add_RelativeDst_NotCwd verifies that cwd does not affect dst
// resolution — even when cwd is the dotfiles repo, .zshrc resolves to $HOME/.zshrc.
func TestE2E_Add_RelativeDst_NotCwd(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"linux/.zshrc": "# zsh\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	t.Chdir(env.RepoDir)

	src := filepath.Join(env.RepoDir, "linux/.zshrc")
	rootCmd.SetArgs([]string{"add",
		"--src", src,
		"--dest", ".zshrc",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	cfg, _ := config.Load(env.ConfigPath)
	want := filepath.Join(env.HomeDir, ".zshrc")
	if cfg.Entries[0].Dst != want {
		t.Errorf("Dst = %q (cwd-relative would be %q)", cfg.Entries[0].Dst, filepath.Join(env.RepoDir, ".zshrc"))
	}
}

// TestE2E_Apply_AbsolutePaths_CreatesSymlink verifies the full add+apply flow
// with absolute src/dst stored in config.
func TestE2E_Apply_AbsolutePaths_CreatesSymlink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"linux/.zshrc": "# zsh\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "linux/.zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", ".zshrc"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	rootCmd.SetArgs([]string{"apply", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}

	env.AssertSymlink(t, ".zshrc", src)
}

// TestE2E_Plan_ShowsAbsolutePaths verifies that plan output shows absolute paths.
func TestE2E_Plan_ShowsAbsolutePaths(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"linux/.zshrc": "# zsh\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "linux/.zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", ".zshrc"})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})

	if !strings.Contains(out, env.RepoDir) {
		t.Errorf("plan output should show absolute src path %q, got:\n%s", env.RepoDir, out)
	}
	if !strings.Contains(out, env.HomeDir) {
		t.Errorf("plan output should show absolute dst path %q, got:\n%s", env.HomeDir, out)
	}
}

// TestE2E_List_ShowsAbsolutePaths verifies list output uses absolute paths.
func TestE2E_List_ShowsAbsolutePaths(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"linux/.zshrc": "# zsh\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "linux/.zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", ".zshrc"})
	_ = rootCmd.Execute()

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"list"})
		_ = rootCmd.Execute()
	})

	if !strings.Contains(out, env.RepoDir) {
		t.Errorf("list must show absolute src path %q, got:\n%s", env.RepoDir, out)
	}
	if !strings.Contains(out, env.HomeDir) {
		t.Errorf("list must show absolute dst path %q, got:\n%s", env.HomeDir, out)
	}
}

// resetAddFlags clears stale cobra flag state between Execute calls in the
// same process. Cobra/pflag does not reset flag values or the Changed map
// between Execute calls, so tests that skip --src/--dest must call this first.
func resetAddFlags() {
	addSrc = ""
	addDest = ""
	if f := addCmd.Flags().Lookup("src"); f != nil {
		f.Changed = false
	}
	if f := addCmd.Flags().Lookup("dest"); f != nil {
		f.Changed = false
	}
}

// TestE2E_Add_PositionalArgs verifies that src and dst can be passed as
// positional arguments without --src / --dest flags.
func TestE2E_Add_PositionalArgs(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")

	resetAddFlags()
	rootCmd.SetArgs([]string{"add", src, dst})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add with positional args: %v", err)
	}

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"list"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, src) {
		t.Errorf("list should contain src %q after positional add, got:\n%s", src, out)
	}
	if !strings.Contains(out, dst) {
		t.Errorf("list should contain dst %q after positional add, got:\n%s", dst, out)
	}
}

// TestE2E_Add_PositionalArgsMissingDest verifies that a single positional arg
// with no --dest flag is rejected with a clear error.
func TestE2E_Add_PositionalArgsMissingDest(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")

	resetAddFlags()
	rootCmd.SetArgs([]string{"add", src})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when dest is missing, got nil")
	}
}

// TestE2E_Apply_NoBackup_Conflict verifies --no-backup deletes the conflicting
// regular file instead of renaming it to a .bk file.
func TestE2E_Apply_NoBackup_Conflict(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	rootCmd.SetArgs([]string{"apply", "--force", "--no-backup"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	env.AssertSymlink(t, ".zshrc", src)
	env.AssertNotExist(t, ".zshrc.bk")
}

// TestE2E_Sort verifies `dotsync sort` reorders entries by Src ascending and
// renumbers IDs to 1..N. Simulates a hand-edited config with out-of-order
// entries and gappy IDs (2, 4, 9).
func TestE2E_Sort(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	srcA := filepath.Join(env.RepoDir, "a/.aaa")
	srcB := filepath.Join(env.RepoDir, "b/.bbb")
	srcC := filepath.Join(env.RepoDir, "c/.ccc")
	env.WriteConfig([]config.Entry{
		{ID: 9, Src: srcC, Dst: filepath.Join(env.HomeDir, ".ccc")},
		{ID: 2, Src: srcA, Dst: filepath.Join(env.HomeDir, ".aaa")},
		{ID: 4, Src: srcB, Dst: filepath.Join(env.HomeDir, ".bbb")},
	})

	rootCmd.SetArgs([]string{"sort"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("sort: %v", err)
	}

	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantSrcs := []string{srcA, srcB, srcC}
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

// TestE2E_Sort_NoChanges asserts that `dotsync sort` on an already
// sorted+normalized config prints "no changes" and does not bump mtime.
func TestE2E_Sort_NoChanges(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	env.WriteConfig([]config.Entry{
		{ID: 1, Src: filepath.Join(env.RepoDir, "a"), Dst: filepath.Join(env.HomeDir, ".a")},
		{ID: 2, Src: filepath.Join(env.RepoDir, "b"), Dst: filepath.Join(env.HomeDir, ".b")},
	})

	info1, err := os.Stat(env.ConfigPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"sort"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "no changes") {
		t.Errorf("expected 'no changes' in output, got: %q", out)
	}

	info2, err := os.Stat(env.ConfigPath)
	if err != nil {
		t.Fatalf("Stat (after): %v", err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("mtime changed for no-op sort: %v -> %v", info1.ModTime(), info2.ModTime())
	}
}

// TestE2E_Sort_RespectsDOTSYNC_CONFIG ensures the override path is the one
// modified (sandbox isolation).
func TestE2E_Sort_RespectsDOTSYNC_CONFIG(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	env.WriteConfig([]config.Entry{
		{ID: 5, Src: filepath.Join(env.RepoDir, "z"), Dst: filepath.Join(env.HomeDir, ".z")},
		{ID: 1, Src: filepath.Join(env.RepoDir, "a"), Dst: filepath.Join(env.HomeDir, ".a")},
	})

	rootCmd.SetArgs([]string{"sort"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("sort: %v", err)
	}

	if _, err := os.Stat(env.ConfigPath); err != nil {
		t.Fatalf("sandbox config missing after sort: %v", err)
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Entries[0].ID != 1 || cfg.Entries[1].ID != 2 {
		t.Errorf("IDs not renumbered: %+v", cfg.Entries)
	}
}

// TestE2E_Apply_NoBackup_Relink verifies --no-backup removes the wrong-target
// symlink instead of renaming it to a .bk file.
func TestE2E_Apply_NoBackup_Relink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{
		"files/.zshrc": "# new\n",
		"old/.zshrc":   "# old target\n",
	})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	wrongTarget := filepath.Join(env.RepoDir, "old/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	if err := os.Symlink(wrongTarget, dst); err != nil {
		t.Fatalf("setup symlink: %v", err)
	}

	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	rootCmd.SetArgs([]string{"apply", "--force", "--no-backup"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	env.AssertSymlink(t, ".zshrc", src)
	env.AssertNotExist(t, ".zshrc.bk")
}
