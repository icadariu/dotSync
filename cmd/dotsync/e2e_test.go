package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/icadariu/dotSync/internal/color"
	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/prompt"
	"github.com/icadariu/dotSync/internal/testenv"
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

// TestHandleVersionFlag verifies only --version prints the version line.
func TestHandleVersionFlag(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	cases := map[string]bool{"--version": true, "-version": false, "version": false}
	for arg, want := range cases {
		os.Args = []string{"dotsync", arg}
		var got bool
		out := captureStdout(func() { got = HandleVersionFlag() })
		if got != want {
			t.Errorf("HandleVersionFlag(%q) = %v, want %v", arg, got, want)
		}
		if want {
			exp := fmt.Sprintf("%s (built %s, commit %s)\n", version, buildTime, commit)
			if out != exp {
				t.Errorf("output for %q = %q, want %q", arg, out, exp)
			}
		} else if out != "" {
			t.Errorf("output for %q = %q, want empty", arg, out)
		}
	}
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
	header, _, _ := strings.Cut(out, "\n")
	if fields := strings.Fields(header); slices.Contains(fields, "OS") || slices.Contains(fields, "TYPE") {
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

// TestE2E_Plan_EscapesControlBytes verifies control bytes in file content and
// in paths never reach the terminal raw.
func TestE2E_Plan_EscapesControlBytes(t *testing.T) {
	env := testenv.New(t)
	prev := color.Enabled
	color.Enabled = false
	t.Cleanup(func() { color.Enabled = prev })
	env.WriteRepo(map[string]string{"files/evil": "\x1b[6A\x1b[2K fake line\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/evil")
	dst := filepath.Join(env.HomeDir, "d\x1b[2Kst")
	if err := os.WriteFile(dst, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	oldErr := os.Stderr
	er, ew, _ := os.Pipe()
	os.Stderr = ew
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan", "--no-pager"})
		_ = rootCmd.Execute()
	})
	ew.Close()
	os.Stderr = oldErr
	errOut, _ := io.ReadAll(er)
	all := out + string(errOut)
	if strings.Contains(all, "\x1b") {
		t.Errorf("raw ESC in plan output: %q", all)
	}
	if !strings.Contains(all, "^[") {
		t.Errorf("expected escaped ESC (^[) in plan output: %q", all)
	}
}

// TestE2E_Apply_PromptEscapesDiff verifies the conflict prompt's diff is escaped.
func TestE2E_Apply_PromptEscapesDiff(t *testing.T) {
	env := testenv.New(t)
	prev := color.Enabled
	color.Enabled = false
	t.Cleanup(func() { color.Enabled = prev })
	env.WriteRepo(map[string]string{"files/.zshrc": "x\x1b[2Ky\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	prompt.Stdin = strings.NewReader("n\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"apply"})
		_ = rootCmd.Execute()
	})
	if strings.Contains(out, "\x1b") {
		t.Errorf("raw ESC in apply output: %q", out)
	}
	if !strings.Contains(out, "^[[2K") {
		t.Errorf("expected escaped diff content in prompt output: %q", out)
	}
}

// pagerSetup registers one creatable .zshrc entry, forces a TTY stdout and
// points DOTSYNC_PAGER at a fake pager that writes stdin to the returned file.
func pagerSetup(t *testing.T, pager string) (env *testenv.Env, pagedFile string) {
	t.Helper()
	env = testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	pagedFile = filepath.Join(env.HomeDir, "paged.txt")
	if pager == "" {
		pager = "cat > " + pagedFile
	}
	t.Setenv("DOTSYNC_PAGER", pager)
	old := stdoutIsTTY
	stdoutIsTTY = func() bool { return true }
	t.Cleanup(func() { stdoutIsTTY = old })
	rootCmd.SetArgs([]string{"add", "--src", filepath.Join(env.RepoDir, "files/.zshrc"), "--dest", filepath.Join(env.HomeDir, ".zshrc")})
	_ = rootCmd.Execute()
	return env, pagedFile
}

// TestE2E_Plan_Pager_PagesChanges verifies plan output goes through the pager on a TTY.
func TestE2E_Plan_Pager_PagesChanges(t *testing.T) {
	_, paged := pagerSetup(t, "")
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	got, err := os.ReadFile(paged)
	if err != nil || !strings.Contains(string(got), "+ create") {
		t.Errorf("pager file missing '+ create' (err=%v):\n%s", err, got)
	}
	if strings.Contains(out, "+ create") {
		t.Errorf("stdout must not contain paged output:\n%s", out)
	}
}

// TestE2E_Plan_Pager_NoPagerFlag verifies --no-pager prints to stdout and skips the pager.
func TestE2E_Plan_Pager_NoPagerFlag(t *testing.T) {
	_, paged := pagerSetup(t, "")
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan", "--no-pager"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "+ create") {
		t.Errorf("stdout missing '+ create':\n%s", out)
	}
	if _, err := os.Stat(paged); err == nil {
		t.Error("pager ran despite --no-pager")
	}
}

// TestE2E_Plan_Pager_NotTTY verifies no pager is used when stdout is not a TTY.
func TestE2E_Plan_Pager_NotTTY(t *testing.T) {
	_, paged := pagerSetup(t, "")
	stdoutIsTTY = func() bool { return false }
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "+ create") {
		t.Errorf("stdout missing '+ create':\n%s", out)
	}
	if _, err := os.Stat(paged); err == nil {
		t.Error("pager ran although stdout is not a TTY")
	}
}

// TestE2E_Plan_Pager_NoChanges verifies "no changes" is printed to stdout, not paged.
func TestE2E_Plan_Pager_NoChanges(t *testing.T) {
	_, paged := pagerSetup(t, "")
	rootCmd.SetArgs([]string{"apply", "--force"})
	_ = rootCmd.Execute()
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if out != "no changes\n" {
		t.Errorf("plan output = %q, want %q", out, "no changes\n")
	}
	if _, err := os.Stat(paged); err == nil {
		t.Error("pager ran although there are no changes")
	}
}

// TestE2E_Plan_Pager_MissingPagerFallsBack verifies output reaches stdout when the pager cannot run.
func TestE2E_Plan_Pager_MissingPagerFallsBack(t *testing.T) {
	pagerSetup(t, "/nonexistent/dotsync-pager")
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "+ create") {
		t.Errorf("stdout missing '+ create' after pager failure:\n%s", out)
	}
}

// TestE2E_Plan_Pager_ErrorStillReturned verifies plan still returns its error summary when paging.
func TestE2E_Plan_Pager_ErrorStillReturned(t *testing.T) {
	env, paged := pagerSetup(t, "")
	env.WriteRepo(map[string]string{"files/.bashrc": "# bash\n"})
	bashSrc := filepath.Join(env.RepoDir, "files/.bashrc")
	rootCmd.SetArgs([]string{"add", "--src", bashSrc, "--dest", filepath.Join(env.HomeDir, ".bashrc")})
	_ = rootCmd.Execute()
	if err := os.Remove(bashSrc); err != nil {
		t.Fatal(err)
	}
	var err error
	captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		err = rootCmd.Execute()
	})
	if err == nil || !strings.Contains(err.Error(), "entries have errors") {
		t.Errorf("plan err = %v, want 'entries have errors'", err)
	}
	got, rerr := os.ReadFile(paged)
	if rerr != nil || !strings.Contains(string(got), "+ create") {
		t.Errorf("pager file missing '+ create' (err=%v):\n%s", rerr, got)
	}
}

// TestPagerCommand verifies pager resolution order and the cat/less special cases.
func TestPagerCommand(t *testing.T) {
	cases := []struct {
		name, dotsync, pager, want string
	}{
		{"none set", "", "", "less -FRX"},
		{"PAGER less", "", "less", "less -FRX"},
		{"PAGER custom", "", "most -s", "most -s"},
		{"DOTSYNC_PAGER beats PAGER", "most -s", "less", "most -s"},
		{"DOTSYNC_PAGER cat", "cat", "", ""},
		{"PAGER cat", "", "cat", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range map[string]string{"DOTSYNC_PAGER": c.dotsync, "PAGER": c.pager} {
				t.Setenv(k, v) // registers restore of the original value
				if v == "" {
					os.Unsetenv(k)
				}
			}
			if got := pagerCommand(); got != c.want {
				t.Errorf("pagerCommand() = %q, want %q", got, c.want)
			}
		})
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

// addAndApplyZshrc registers one entry and applies it with --force.
func addAndApplyZshrc(t *testing.T) {
	t.Helper()
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	rootCmd.SetArgs([]string{"add", "--src", filepath.Join(env.RepoDir, "files/.zshrc"), "--dest", filepath.Join(env.HomeDir, ".zshrc")})
	_ = rootCmd.Execute()
	rootCmd.SetArgs([]string{"apply", "--force"})
	_ = rootCmd.Execute()
}

func TestE2E_Plan_NoChanges(t *testing.T) {
	addAndApplyZshrc(t)
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan"})
		_ = rootCmd.Execute()
	})
	if out != "no changes\n" {
		t.Errorf("plan output = %q, want %q", out, "no changes\n")
	}
}

func TestE2E_Plan_Verbose_NoNoChangesLine(t *testing.T) {
	addAndApplyZshrc(t)
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan", "--verbose"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "= unchanged") {
		t.Errorf("plan -v should list = unchanged:\n%s", out)
	}
	if strings.Contains(out, "no changes") {
		t.Errorf("plan -v must not print 'no changes':\n%s", out)
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
// TestE2E_Delete_PickerThenConfirm_PipedInput verifies the id picker and the
// y/n confirmation each read their own line of piped input.
func TestE2E_Delete_PickerThenConfirm_PipedInput(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	rootCmd.SetArgs([]string{"add",
		"--src", filepath.Join(env.RepoDir, "files/.zshrc"),
		"--dest", filepath.Join(env.HomeDir, ".zshrc"),
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	prompt.Stdin = strings.NewReader("1\ny\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	rootCmd.SetArgs([]string{"delete", "99"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 0 {
		t.Errorf("expected entry to be deleted, got %d entries", len(cfg.Entries))
	}
}

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

// TestE2E_Delete_NoID_ForcePicksFromList verifies delete without an id lists entries
// and reads the id; --force skips the y/n confirmation.
func TestE2E_Delete_NoID_ForcePicksFromList(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	prompt.Stdin = strings.NewReader("1\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	rootCmd.SetArgs([]string{"delete", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete with missing id: %v", err)
	}
	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 0 {
		t.Errorf("expected entry to be deleted, got %d entries", len(cfg.Entries))
	}
}

// TestE2E_Delete_ForceMissingID_Errors verifies --force with an unknown id errors
// instead of falling back to the picker.
func TestE2E_Delete_ForceMissingID_Errors(t *testing.T) {
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
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no entry with id 99") {
		t.Fatalf("expected 'no entry with id 99' error, got %v", err)
	}
	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 1 {
		t.Errorf("expected 1 entry kept, got %d", len(cfg.Entries))
	}
}

// TestE2E_Delete_LeavesForeignSymlink verifies a dst symlink pointing somewhere other
// than src is left in place while the entry is removed.
func TestE2E_Delete_LeavesForeignSymlink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	env.WriteHome(map[string]string{"other/.zshrc": "mine\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()
	foreign := filepath.Join(env.HomeDir, "other/.zshrc")
	if err := os.Symlink(foreign, dst); err != nil {
		t.Fatal(err)
	}

	rootCmd.SetArgs([]string{"delete", "1", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 0 {
		t.Errorf("expected 0 entries after delete, got %d", len(cfg.Entries))
	}
	env.AssertSymlink(t, ".zshrc", foreign)
}

// TestE2E_Delete_UnlinkFailure_ReturnsError verifies an unlink failure surfaces as an
// error while the entry is still removed from the config.
func TestE2E_Delete_UnlinkFailure_ReturnsError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	env.WriteHome(map[string]string{"ro/.keep": ""})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	roDir := filepath.Join(env.HomeDir, "ro")
	dst := filepath.Join(roDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()
	if err := os.Symlink(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	rootCmd.SetArgs([]string{"delete", "1", "--force"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when unlink fails")
	}
	cfg, _ := config.Load(env.ConfigPath)
	if len(cfg.Entries) != 0 {
		t.Errorf("expected entry removed from config, got %d entries", len(cfg.Entries))
	}
	if _, err := os.Lstat(dst); err != nil {
		t.Errorf("symlink should still exist: %v", err)
	}
}

// TestE2E_Edit_EditorWithArgs verifies $EDITOR may carry arguments (e.g. "code --wait").
func TestE2E_Edit_EditorWithArgs(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n", "files/.vimrc": "\" vim\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	zshSrc := filepath.Join(env.RepoDir, "files/.zshrc")
	zshDst := filepath.Join(env.HomeDir, ".zshrc")
	vimSrc := filepath.Join(env.RepoDir, "files/.vimrc")
	vimDst := filepath.Join(env.HomeDir, ".vimrc")
	env.WriteConfig([]config.Entry{{ID: 1, Src: zshSrc, Dst: zshDst}})

	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	body := fmt.Sprintf("#!/bin/sh\n[ \"$1\" = \"--wait\" ] || exit 3\nprintf 'id: 1\\nsrc: %s\\ndst: %s\\n' > \"$2\"\n", vimSrc, vimDst)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script+" --wait")

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("edit with args in EDITOR: %v", err)
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Entries[0].Src != vimSrc || cfg.Entries[0].Dst != vimDst {
		t.Errorf("entry not updated: %+v", cfg.Entries[0])
	}
}

// TestE2E_Edit_UnknownKeyRejected verifies a typo'd key in the edited entry
// fails the edit instead of being silently dropped, and the config is untouched.
func TestE2E_Edit_UnknownKeyRejected(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	env.WriteConfig([]config.Entry{{ID: 1, Src: src, Dst: dst}})
	before, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'hostz: [laptop]' >> \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)

	rootCmd.SetArgs([]string{"edit", "1"})
	err = rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "hostz") {
		t.Fatalf("want error naming hostz, got %v", err)
	}
	after, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("config changed despite rejected edit:\n%s", after)
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
// command and asserts that delete (and its del alias), edit, apply and plan
// return existing entry IDs.
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

	for _, sub := range []string{"delete", "del", "edit", "apply", "plan"} {
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
		"--dest", "~/.zshrc",
	})
	var execErr error
	out := captureStdout(func() { execErr = rootCmd.Execute() })
	if execErr != nil {
		t.Fatalf("add: %v", execErr)
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
	if want := "added 1: " + wantSrc + " -> " + wantDst + "\n"; !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want it to contain %q", out, want)
	}
	if cfg.Entries[0].Src != wantSrc {
		t.Errorf("Src = %q, want %q", cfg.Entries[0].Src, wantSrc)
	}
	if cfg.Entries[0].Dst != wantDst {
		t.Errorf("Dst = %q, want %q", cfg.Entries[0].Dst, wantDst)
	}
}

// A dst ending in / names the directory to link into, like ln -s: the link
// keeps src's basename, whether the directory exists or not.
func TestE2E_Add_TrailingSlashDst_LinksInside(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"dir1/dir2/file1": "one\n", "dir1/file2": "two\n"})
	env.WriteHome(map[string]string{"some_dir/other": "keep\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	t.Chdir(env.RepoDir)

	rootCmd.SetArgs([]string{"add", "dir1/dir2/file1", "~/some_dir/"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add into existing dir: %v", err)
	}
	resetFlags()
	rootCmd.SetArgs([]string{"add", "--src", "dir1/file2", "--dest", "~/new_dir/"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add into missing dir: %v", err)
	}

	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{filepath.Join(env.HomeDir, "some_dir/file1"), filepath.Join(env.HomeDir, "new_dir/file2")}
	if len(cfg.Entries) != 2 || cfg.Entries[0].Dst != want[0] || cfg.Entries[1].Dst != want[1] {
		t.Fatalf("entries = %+v, want dsts %q", cfg.Entries, want)
	}

	resetFlags()
	rootCmd.SetArgs([]string{"apply", "-f"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	env.AssertSymlink(t, "some_dir/file1", filepath.Join(env.RepoDir, "dir1/dir2/file1"))
	env.AssertSymlink(t, "new_dir/file2", filepath.Join(env.RepoDir, "dir1/file2"))
	env.AssertRegular(t, "some_dir/other", "keep\n")
}

// Without a trailing slash, a file src over an existing directory is
// ambiguous, so add asks: link into it, replace it, or cancel.
func TestE2E_Add_FileOverExistingDir_Prompts(t *testing.T) {
	for _, tc := range []struct{ answer, wantDst string }{
		{"\n", "some_dir/file1"},
		{"i\n", "some_dir/file1"},
		{"r\n", "some_dir"},
		{"c\n", ""},
	} {
		env := testenv.New(t)
		env.WriteRepo(map[string]string{"file1": "one\n"})
		env.WriteHome(map[string]string{"some_dir/other": "keep\n"})
		env.WriteConfig(nil)
		t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
		prompt.Stdin = strings.NewReader(tc.answer)
		t.Cleanup(func() { prompt.Stdin = os.Stdin })

		rootCmd.SetArgs([]string{"add", filepath.Join(env.RepoDir, "file1"), "~/some_dir"})
		var execErr error
		out := captureStdout(func() { execErr = rootCmd.Execute() })
		if execErr != nil {
			t.Fatalf("answer %q: add: %v", tc.answer, execErr)
		}
		if !strings.Contains(out, "is a directory") {
			t.Errorf("answer %q: want a prompt about the directory, got %q", tc.answer, out)
		}
		cfg, err := config.Load(env.ConfigPath)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if tc.wantDst == "" {
			if len(cfg.Entries) != 0 {
				t.Errorf("answer %q: cancel must not save, got %+v", tc.answer, cfg.Entries)
			}
			continue
		}
		if want := filepath.Join(env.HomeDir, tc.wantDst); len(cfg.Entries) != 1 || cfg.Entries[0].Dst != want {
			t.Errorf("answer %q: entries = %+v, want dst %s", tc.answer, cfg.Entries, want)
		}
	}
}

func TestE2E_Add_FileOverExistingDir_NoInput_Errors(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"file1": "one\n"})
	env.WriteHome(map[string]string{"some_dir/other": "keep\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	prompt.Stdin = strings.NewReader("")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	rootCmd.SetArgs([]string{"add", filepath.Join(env.RepoDir, "file1"), "~/some_dir"})
	var execErr error
	captureStdout(func() { execErr = rootCmd.Execute() })
	if execErr == nil || !strings.Contains(execErr.Error(), "end dst with /") {
		t.Fatalf("want an error suggesting a trailing slash, got %v", execErr)
	}
}

// Any other existing dst is saved without a prompt, with a warning on stderr
// (so scripts see it) that apply will back it up; a dst that is already the
// link to src gets none.
func TestE2E_Add_ExistingDst_Notes(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{".zshrc": "new\n", "nvim/init.lua": "new\n", ".vimrc": "v\n"})
	env.WriteHome(map[string]string{".zshrc": "old\n", ".config/nvim/init.lua": "old\n"})
	env.MakeSymlink(filepath.Join(env.RepoDir, ".vimrc"), ".vimrc")
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	prompt.Stdin = strings.NewReader("") // any prompt would fail the add
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	for _, tc := range []struct {
		src, dst string
		note     bool
	}{
		{".zshrc", "~/.zshrc", true},
		{"nvim", "~/.config/nvim", true},
		{".vimrc", "~/.vimrc", false},
	} {
		resetFlags()
		rootCmd.SetArgs([]string{"add", filepath.Join(env.RepoDir, tc.src), tc.dst})
		var execErr error
		var stderr string
		out := captureStdout(func() { stderr = captureStderr(func() { execErr = rootCmd.Execute() }) })
		if execErr != nil {
			t.Fatalf("add %s: %v", tc.dst, execErr)
		}
		if got := strings.Contains(stderr, "warning: "+filepath.Join(env.HomeDir, strings.TrimPrefix(tc.dst, "~/"))+" already exists; apply will move it to a backup"); got != tc.note {
			t.Errorf("add %s: warning shown = %v, want %v; stderr %q", tc.dst, got, tc.note, stderr)
		}
		if strings.Contains(out, "already exists") {
			t.Errorf("add %s: the warning belongs on stderr, stdout %q", tc.dst, out)
		}
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
		"--dest", "~/.gitconfig",
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
		"--dest", "~/.zshrc",
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

// TestE2E_Add_BareRelativeDst_ResolvedAgainstCWD verifies that a bare relative
// dest (e.g. "f") is resolved against the current working directory, and that
// apply links it to the absolute src even when src was given relative.
func TestE2E_Add_BareRelativeDst_ResolvedAgainstCWD(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"linux/f": "x\n"})
	env.WriteHome(map[string]string{".config/app/.keep": ""})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	cwd := filepath.Join(env.HomeDir, ".config/app")
	t.Chdir(cwd)

	absSrc := filepath.Join(env.RepoDir, "linux/f")
	relSrc, err := filepath.Rel(cwd, absSrc)
	if err != nil {
		t.Fatal(err)
	}
	rootCmd.SetArgs([]string{"add", "--src", relSrc, "--dest", "f"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	cfg, _ := config.Load(env.ConfigPath)
	want := filepath.Join(cwd, "f")
	if cfg.Entries[0].Dst != want {
		t.Errorf("Dst = %q, want %q", cfg.Entries[0].Dst, want)
	}

	rootCmd.SetArgs([]string{"apply", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	env.AssertSymlink(t, ".config/app/f", absSrc)
}

// TestE2E_Add_DotSlashDst_ResolvedAgainstCWD verifies that a dst starting with
// ./ resolves against the current working directory, not $HOME.
func TestE2E_Add_DotSlashDst_ResolvedAgainstCWD(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"linux/.envrc": "x\n"})
	env.WriteHome(map[string]string{"proj/.keep": ""})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	t.Chdir(filepath.Join(env.HomeDir, "proj"))

	rootCmd.SetArgs([]string{"add",
		"--src", filepath.Join(env.RepoDir, "linux/.envrc"),
		"--dest", "./.envrc",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(env.HomeDir, "proj", ".envrc")
	if cfg.Entries[0].Dst != want {
		t.Errorf("Dst = %q, want %q", cfg.Entries[0].Dst, want)
	}
}

// TestE2E_Add_SrcEqualsDst_Rejected verifies add refuses an entry that would
// replace src with a symlink to itself.
func TestE2E_Add_SrcEqualsDst_Rejected(t *testing.T) {
	env := testenv.New(t)
	env.WriteHome(map[string]string{".gitconfig": "precious\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	rootCmd.SetArgs([]string{"add",
		"--src", filepath.Join(env.HomeDir, ".gitconfig"),
		"--dest", "~/.gitconfig",
	})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for src == dst")
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(cfg.Entries))
	}
}

// TestE2E_Edit_RelativePathsResolved verifies edit resolves relative src/dst
// against the current working directory, the same way add does.
func TestE2E_Edit_RelativePathsResolved(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n", "files/.vimrc": "\" vim\n"})
	env.WriteConfig([]config.Entry{{
		ID:  1,
		Src: filepath.Join(env.RepoDir, "files/.zshrc"),
		Dst: filepath.Join(env.HomeDir, ".zshrc"),
	}})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	body := "#!/bin/sh\nprintf 'id: 1\\nsrc: files/.vimrc\\ndst: .vimrc\\n' > \"$1\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)
	t.Chdir(env.RepoDir)

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("edit: %v", err)
	}

	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(env.RepoDir, "files/.vimrc"); cfg.Entries[0].Src != want {
		t.Errorf("Src = %q, want %q", cfg.Entries[0].Src, want)
	}
	if want := filepath.Join(env.RepoDir, ".vimrc"); cfg.Entries[0].Dst != want {
		t.Errorf("Dst = %q, want %q", cfg.Entries[0].Dst, want)
	}
}

// TestE2E_Edit_SrcEqualsDst_Rejected verifies edit refuses an overlapping
// src/dst pair and leaves the stored entry untouched.
func TestE2E_Edit_SrcEqualsDst_Rejected(t *testing.T) {
	env := testenv.New(t)
	env.WriteHome(map[string]string{".gitconfig": "x\n"})
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	origSrc := filepath.Join(env.RepoDir, "files/.zshrc")
	env.WriteConfig([]config.Entry{{ID: 1, Src: origSrc, Dst: filepath.Join(env.HomeDir, ".zshrc")}})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	body := fmt.Sprintf("#!/bin/sh\nprintf 'id: 1\\nsrc: %s\\ndst: %s\\n' > \"$1\"\n",
		filepath.Join(env.HomeDir, ".gitconfig"), filepath.Join(env.HomeDir, ".gitconfig"))
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for src == dst")
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Entries[0].Src != origSrc {
		t.Errorf("Src = %q, want unchanged %q", cfg.Entries[0].Src, origSrc)
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
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", "~/.zshrc"})
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
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", "~/.zshrc"})
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
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", "~/.zshrc"})
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

// TestResetFlags_RunsPerSandbox checks a flag set by one test does not leak
// into the next: testenv.New resets every command's flags.
func TestResetFlags_RunsPerSandbox(t *testing.T) {
	if err := applyCmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	testenv.New(t)
	if applyForce || applyCmd.Flags().Changed("force") {
		t.Error("apply --force leaked past testenv.New")
	}
}

// resetFlags restores every command's flags to their defaults. Cobra/pflag
// keep parsed values and the Changed bit across Execute calls in one process,
// so a flag set by one test (e.g. apply --force) would otherwise leak into the
// next. testenv.New runs it at the start of every e2e test.
func resetFlags() {
	reset := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(f *pflag.Flag) {
			// Set(DefValue) on a slice flag would append a literal "[]".
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
				f.Changed = false
				return
			}
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	reset(rootCmd.PersistentFlags())
	for _, c := range rootCmd.Commands() {
		reset(c.Flags())
	}
}

func init() { testenv.OnNew = resetFlags }

// TestE2E_Add_PositionalArgs verifies that src and dst can be passed as
// positional arguments without --src / --dest flags.
func TestE2E_Add_PositionalArgs(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")

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

	rootCmd.SetArgs([]string{"add", src})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when dest is missing, got nil")
	}
}

func TestE2E_Add_Adopt_MovesAndLinks(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	env.WriteHome(map[string]string{".tmux.conf": "set -g mouse on\n"})

	src := filepath.Join(env.RepoDir, "tmux/.tmux.conf")
	rootCmd.SetArgs([]string{"add", "--adopt", src, "~/.tmux.conf"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add --adopt: %v", err)
	}

	env.AssertSymlink(t, ".tmux.conf", src)
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading adopted src: %v", err)
	}
	if string(got) != "set -g mouse on\n" {
		t.Errorf("src content = %q, want %q", got, "set -g mouse on\n")
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(cfg.Entries))
	}
	if cfg.Entries[0].Src != src {
		t.Errorf("entry Src = %q, want %q", cfg.Entries[0].Src, src)
	}
	if want := filepath.Join(env.HomeDir, ".tmux.conf"); cfg.Entries[0].Dst != want {
		t.Errorf("entry Dst = %q, want %q", cfg.Entries[0].Dst, want)
	}
}

func TestE2E_Add_Adopt_TrailingSlashDst(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	env.WriteHome(map[string]string{".tmux.conf": "set -g mouse on\n"})

	src := filepath.Join(env.RepoDir, "tmux/.tmux.conf")
	rootCmd.SetArgs([]string{"add", "--adopt", src, "~/"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add --adopt: %v", err)
	}
	env.AssertSymlink(t, ".tmux.conf", src)
}

func TestE2E_Add_Adopt_SrcExists_Errors(t *testing.T) {
	env := testenv.New(t)
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	env.WriteRepo(map[string]string{"tmux/.tmux.conf": "repo\n"})
	env.WriteHome(map[string]string{".tmux.conf": "home\n"})

	src := filepath.Join(env.RepoDir, "tmux/.tmux.conf")
	rootCmd.SetArgs([]string{"add", "--adopt", src, "~/.tmux.conf"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when src already exists, got nil")
	}

	env.AssertRegular(t, ".tmux.conf", "home\n")
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(cfg.Entries))
	}
}

func TestE2E_Add_Adopt_DstMissing_Errors(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	src := filepath.Join(env.RepoDir, "tmux/.tmux.conf")
	rootCmd.SetArgs([]string{"add", "--adopt", src, "~/.tmux.conf"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when dst is missing, got nil")
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("src should not exist, Lstat err = %v", err)
	}
}

func TestE2E_Add_Adopt_DstSymlink_Errors(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	env.WriteRepo(map[string]string{"other": "x\n"})
	other := filepath.Join(env.RepoDir, "other")
	env.MakeSymlink(other, ".tmux.conf")

	src := filepath.Join(env.RepoDir, "tmux/.tmux.conf")
	rootCmd.SetArgs([]string{"add", "--adopt", src, "~/.tmux.conf"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when dst is a symlink, got nil")
	}
	env.AssertSymlink(t, ".tmux.conf", other)
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("src should not exist, Lstat err = %v", err)
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

// TestApply_ExistingBackup_Prompt_No_KeepsAll verifies the per-entry prompt
// warns that an existing backup will be overwritten and that declining leaves
// both the file and its backup untouched.
func TestApply_ExistingBackup_Prompt_No_KeepsAll(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.vimrc": "new\n"})
	env.WriteHome(map[string]string{".vimrc": "cur\n", ".vimrc.bk": "old\n"})
	env.WriteConfig([]config.Entry{{ID: 1, Src: filepath.Join(env.RepoDir, "files/.vimrc"), Dst: filepath.Join(env.HomeDir, ".vimrc")}})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	prompt.Stdin = strings.NewReader("n\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	var err error
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"apply"})
		err = rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out, "will be overwritten") {
		t.Errorf("prompt should mention the overwrite, got:\n%s", out)
	}
	env.AssertRegular(t, ".vimrc", "cur\n")
	env.AssertRegular(t, ".vimrc.bk", "old\n")
}

// Overwriting a backup is irreversible, so a bare Enter must not do it.
func TestApply_ExistingBackup_BareEnter_KeepsAll(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.vimrc": "new\n"})
	env.WriteHome(map[string]string{".vimrc": "cur\n", ".vimrc.bk": "old\n"})
	env.WriteConfig([]config.Entry{{ID: 1, Src: filepath.Join(env.RepoDir, "files/.vimrc"), Dst: filepath.Join(env.HomeDir, ".vimrc")}})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	prompt.Stdin = strings.NewReader("\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	captureStdout(func() {
		rootCmd.SetArgs([]string{"apply"})
		_ = rootCmd.Execute()
	})
	env.AssertRegular(t, ".vimrc", "cur\n")
	env.AssertRegular(t, ".vimrc.bk", "old\n")
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

// TestE2E_Apply_ErrorExitsNonZero verifies apply returns an error when an
// entry fails, while still processing the remaining entries.
func TestE2E_Apply_ErrorExitsNonZero(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.present": "content\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	present := filepath.Join(env.RepoDir, "files/.present")
	env.WriteConfig([]config.Entry{
		{ID: 1, Src: filepath.Join(env.RepoDir, "missing"), Dst: filepath.Join(env.HomeDir, ".missing")},
		{ID: 2, Src: present, Dst: filepath.Join(env.HomeDir, ".present")},
	})

	rootCmd.SetArgs([]string{"apply", "-f"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected apply to return an error")
	}
	env.AssertSymlink(t, ".present", present)
}

// TestE2E_Plan_ErrorExitsNonZero verifies plan returns an error when an entry
// has an error status.
func TestE2E_Plan_ErrorExitsNonZero(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	env.WriteConfig([]config.Entry{
		{ID: 1, Src: filepath.Join(env.RepoDir, "missing"), Dst: filepath.Join(env.HomeDir, ".missing")},
	})

	rootCmd.SetArgs([]string{"plan"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected plan to return an error")
	}
}

// TestE2E_Apply_SelectedIDs verifies apply only touches the listed entry IDs.
func TestE2E_Apply_SelectedIDs(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	env.WriteConfig([]config.Entry{
		{ID: 1, Src: filepath.Join(env.RepoDir, "files/.a"), Dst: filepath.Join(env.HomeDir, ".a")},
		{ID: 2, Src: filepath.Join(env.RepoDir, "files/.b"), Dst: filepath.Join(env.HomeDir, ".b")},
	})

	rootCmd.SetArgs([]string{"apply", "-f", "2"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	env.AssertNotExist(t, ".a")
	env.AssertSymlink(t, ".b", filepath.Join(env.RepoDir, "files/.b"))
}

// TestE2E_Plan_SelectedIDs verifies plan only reports the listed entry IDs.
func TestE2E_Plan_SelectedIDs(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	env.WriteConfig([]config.Entry{
		{ID: 1, Src: filepath.Join(env.RepoDir, "files/.a"), Dst: filepath.Join(env.HomeDir, ".a")},
		{ID: 2, Src: filepath.Join(env.RepoDir, "files/.b"), Dst: filepath.Join(env.HomeDir, ".b")},
	})

	var execErr error
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"plan", "1"})
		execErr = rootCmd.Execute()
	})
	if execErr != nil {
		t.Fatal(execErr)
	}
	if !strings.Contains(out, filepath.Join(env.HomeDir, ".a")) {
		t.Errorf("plan output missing .a:\n%s", out)
	}
	if strings.Contains(out, filepath.Join(env.HomeDir, ".b")) {
		t.Errorf("plan output unexpectedly contains .b:\n%s", out)
	}
}

// TestE2E_Apply_UnknownID_Errors verifies an unknown ID fails before any change.
func TestE2E_Apply_UnknownID_Errors(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	env.WriteConfig([]config.Entry{
		{ID: 1, Src: filepath.Join(env.RepoDir, "files/.a"), Dst: filepath.Join(env.HomeDir, ".a")},
		{ID: 2, Src: filepath.Join(env.RepoDir, "files/.b"), Dst: filepath.Join(env.HomeDir, ".b")},
	})

	rootCmd.SetArgs([]string{"apply", "-f", "9"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected apply with unknown id to return an error")
	}
	env.AssertNotExist(t, ".a")
	env.AssertNotExist(t, ".b")
}

// TestE2E_Completion_ApplyExcludesGivenIDs verifies apply completion skips IDs
// already on the command line.
func TestE2E_Completion_ApplyExcludesGivenIDs(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	env.WriteConfig([]config.Entry{
		{ID: 1, Src: filepath.Join(env.RepoDir, "files/.a"), Dst: filepath.Join(env.HomeDir, ".a")},
		{ID: 2, Src: filepath.Join(env.RepoDir, "files/.b"), Dst: filepath.Join(env.HomeDir, ".b")},
	})

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"__complete", "apply", "1", ""})
		_ = rootCmd.Execute()
	})
	has1, has2 := false, false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "1\t") {
			has1 = true
		}
		if strings.HasPrefix(line, "2\t") {
			has2 = true
		}
	}
	if has1 || !has2 {
		t.Errorf("expected only id 2 suggested, got:\n%s", out)
	}
}

// TestE2E_Edit_DollarHomeSrc verifies edit expands a literal $HOME in src.
func TestE2E_Edit_DollarHomeSrc(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n", "files/.vimrc": "\" vim\n"})
	env.WriteConfig([]config.Entry{{
		ID:  1,
		Src: filepath.Join(env.RepoDir, "files/.zshrc"),
		Dst: filepath.Join(env.HomeDir, ".zshrc"),
	}})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	body := fmt.Sprintf("#!/bin/sh\nprintf 'id: 1\\nsrc: $HOME/../repo/files/.vimrc\\ndst: %s\\n' > \"$1\"\n", filepath.Join(env.HomeDir, ".vimrc"))
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("edit: %v", err)
	}

	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(env.RepoDir, "files/.vimrc"); cfg.Entries[0].Src != want {
		t.Errorf("Src = %q, want %q", cfg.Entries[0].Src, want)
	}
}

// TestE2E_Add_SrcFlagWithPositionalDst verifies a positional arg fills dst
// when only --src is given.
func TestE2E_Add_SrcFlagWithPositionalDst(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	rootCmd.SetArgs([]string{"add", "--src", filepath.Join(env.RepoDir, "files/.zshrc"), "~/.y"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(cfg.Entries))
	}
	if want := filepath.Join(env.HomeDir, ".y"); cfg.Entries[0].Dst != want {
		t.Errorf("Dst = %q, want %q", cfg.Entries[0].Dst, want)
	}
}

// TestE2E_Add_ExtraPositionalArg_Errors verifies leftover positional args
// are rejected and nothing is added.
func TestE2E_Add_ExtraPositionalArg_Errors(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	rootCmd.SetArgs([]string{"add",
		"--src", filepath.Join(env.RepoDir, "files/.zshrc"),
		"--dest", filepath.Join(env.HomeDir, ".zshrc"),
		"extra",
	})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for extra positional arg")
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(cfg.Entries))
	}
}

// TestE2E_MissingConfig_CommandsError checks commands other than add fail on a
// missing config and do not create it.
func TestE2E_MissingConfig_CommandsError(t *testing.T) {
	env := testenv.New(t)
	typo := filepath.Join(env.HomeDir, "typo.yaml")
	t.Setenv("DOTSYNC_CONFIG", typo)

	for _, args := range [][]string{{"list"}, {"plan"}, {"apply", "--force"}} {
		var err error
		captureStdout(func() {
			rootCmd.SetArgs(args)
			err = rootCmd.Execute()
		})
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("%v: want error containing %q, got %v", args, "does not exist", err)
		}
	}
	if _, err := os.Stat(typo); !os.IsNotExist(err) {
		t.Errorf("config %s should not have been created (stat err: %v)", typo, err)
	}
}

// TestE2E_Add_MissingConfig_CreatesIt checks add bootstraps a missing config
// with the default backup suffix.
func TestE2E_Add_MissingConfig_CreatesIt(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")

	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Entries) != 1 {
		t.Errorf("entries = %d, want 1", len(cfg.Entries))
	}
	if cfg.BackupSuffix != ".bk" {
		t.Errorf("BackupSuffix = %q, want %q", cfg.BackupSuffix, ".bk")
	}
}

// badIDsConfig writes a hand-edited config (src-ordered) whose ids are 3,3 and
// missing; WriteConfig cannot express a missing id.
func badIDsConfig(t *testing.T, env *testenv.Env) {
	t.Helper()
	content := "version: 1\nentries:\n" +
		"  - id: 3\n    src: " + filepath.Join(env.RepoDir, "files/.a") + "\n    dst: " + filepath.Join(env.HomeDir, ".a") + "\n" +
		"  - id: 3\n    src: " + filepath.Join(env.RepoDir, "files/.b") + "\n    dst: " + filepath.Join(env.HomeDir, ".b") + "\n" +
		"  - src: " + filepath.Join(env.RepoDir, "files/.c") + "\n    dst: " + filepath.Join(env.HomeDir, ".c") + "\n"
	if err := os.WriteFile(env.ConfigPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestE2E_Apply_DuplicateIDsAllAddressable: duplicate/missing on-disk ids are
// renumbered on load, so every entry can be selected by its position.
func TestE2E_Apply_DuplicateIDsAllAddressable(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n", "files/.c": "c\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	badIDsConfig(t, env)

	rootCmd.SetArgs([]string{"apply", "2", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply 2: %v", err)
	}
	env.AssertSymlink(t, ".b", filepath.Join(env.RepoDir, "files/.b"))
	env.AssertNotExist(t, ".a")
	env.AssertNotExist(t, ".c")

	resetFlags() // reset flags between the two Execute calls
	rootCmd.SetArgs([]string{"apply", "3", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("apply 3: %v", err)
	}
	env.AssertSymlink(t, ".c", filepath.Join(env.RepoDir, "files/.c"))
	env.AssertNotExist(t, ".a")
}

// TestE2E_Sort_RewritesBadIDs: sort must still notice bad on-disk ids even
// though Load renumbers in memory, and rewrite the file.
func TestE2E_Sort_RewritesBadIDs(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	badIDsConfig(t, env)

	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"sort"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "sorted") {
		t.Errorf("expected 'sorted' in output, got: %q", out)
	}

	var got config.Config
	testenv.ReadYAML(t, env.ConfigPath, &got)
	if len(got.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(got.Entries))
	}
	for i, e := range got.Entries {
		if e.ID != i+1 {
			t.Errorf("Entries[%d].ID = %d, want %d", i, e.ID, i+1)
		}
	}
}

// unlinkFixture writes a repo with files/.a and files/.b, a config with both
// entries (dst in HomeDir) and returns their src/dst paths.
func unlinkFixture(t *testing.T) (env *testenv.Env, srcA, dstA, srcB, dstB string) {
	t.Helper()
	env = testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	srcA, dstA = filepath.Join(env.RepoDir, "files/.a"), filepath.Join(env.HomeDir, ".a")
	srcB, dstB = filepath.Join(env.RepoDir, "files/.b"), filepath.Join(env.HomeDir, ".b")
	env.WriteConfig([]config.Entry{{ID: 1, Src: srcA, Dst: dstA}, {ID: 2, Src: srcB, Dst: dstB}})
	return env, srcA, dstA, srcB, dstB
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// TestE2E_Unlink_AllForce verifies no ids unlinks every entry and keeps the sources.
func TestE2E_Unlink_AllForce(t *testing.T) {
	env, srcA, dstA, srcB, dstB := unlinkFixture(t)
	mustSymlink(t, srcA, dstA)
	mustSymlink(t, srcB, dstB)

	rootCmd.SetArgs([]string{"unlink", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	env.AssertNotExist(t, ".a")
	env.AssertNotExist(t, ".b")
	for _, s := range []string{srcA, srcB} {
		if _, err := os.Stat(s); err != nil {
			t.Errorf("src must be untouched: %v", err)
		}
	}
}

// TestE2E_Unlink_ByID verifies only the listed entry is unlinked.
func TestE2E_Unlink_ByID(t *testing.T) {
	env, srcA, dstA, srcB, dstB := unlinkFixture(t)
	mustSymlink(t, srcA, dstA)
	mustSymlink(t, srcB, dstB)

	rootCmd.SetArgs([]string{"unlink", "--force", "2"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	env.AssertSymlink(t, ".a", srcA)
	env.AssertNotExist(t, ".b")
}

// TestE2E_Unlink_LeavesRealFile verifies a real file at dst is kept and is not an error.
func TestE2E_Unlink_LeavesRealFile(t *testing.T) {
	env, _, _, _, _ := unlinkFixture(t)
	env.WriteHome(map[string]string{".a": "real\n"})

	rootCmd.SetArgs([]string{"unlink", "--force", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	env.AssertRegular(t, ".a", "real\n")
}

// TestE2E_Unlink_LeavesForeignSymlink verifies a link pointing elsewhere is kept.
func TestE2E_Unlink_LeavesForeignSymlink(t *testing.T) {
	env, _, dstA, _, _ := unlinkFixture(t)
	env.WriteHome(map[string]string{"other/.a": "mine\n"})
	foreign := filepath.Join(env.HomeDir, "other/.a")
	mustSymlink(t, foreign, dstA)

	rootCmd.SetArgs([]string{"unlink", "--force", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	env.AssertSymlink(t, ".a", foreign)
}

// TestE2E_Unlink_RestoreSingleBackup verifies --restore only restores
// dst+suffix and ignores numbered leftovers from older versions.
func TestE2E_Unlink_RestoreSingleBackup(t *testing.T) {
	env, srcA, dstA, _, _ := unlinkFixture(t)
	env.WriteHome(map[string]string{".a.bk": "old0\n", ".a.bk.2": "old2\n"})
	mustSymlink(t, srcA, dstA)

	rootCmd.SetArgs([]string{"unlink", "--force", "--restore", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	env.AssertRegular(t, ".a", "old0\n")
	env.AssertNotExist(t, ".a.bk")
	env.AssertRegular(t, ".a.bk.2", "old2\n")
}

// TestE2E_Unlink_RestoreNoBackup verifies --restore without a backup just unlinks.
func TestE2E_Unlink_RestoreNoBackup(t *testing.T) {
	env, srcA, dstA, _, _ := unlinkFixture(t)
	mustSymlink(t, srcA, dstA)

	rootCmd.SetArgs([]string{"unlink", "--force", "--restore", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	env.AssertNotExist(t, ".a")
}

// TestE2E_Unlink_PromptDefaultNo verifies an empty answer cancels and changes nothing.
func TestE2E_Unlink_PromptDefaultNo(t *testing.T) {
	env, srcA, dstA, srcB, dstB := unlinkFixture(t)
	mustSymlink(t, srcA, dstA)
	mustSymlink(t, srcB, dstB)

	prompt.Stdin = strings.NewReader("\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	var execErr error
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"unlink"})
		execErr = rootCmd.Execute()
	})
	if execErr != nil {
		t.Fatalf("unlink: %v", execErr)
	}
	if !strings.Contains(out, "unlink 2 entries?") {
		t.Errorf("output missing question:\n%s", out)
	}
	if !strings.Contains(out, "cancelled") {
		t.Errorf("output missing cancelled:\n%s", out)
	}
	env.AssertSymlink(t, ".a", srcA)
	env.AssertSymlink(t, ".b", srcB)
}

// TestE2E_Unlink_ConfigUntouched verifies the config file is byte-identical afterwards.
func TestE2E_Unlink_ConfigUntouched(t *testing.T) {
	env, srcA, dstA, _, _ := unlinkFixture(t)
	mustSymlink(t, srcA, dstA)
	before, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	rootCmd.SetArgs([]string{"unlink", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	after, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("config changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestE2E_Unlink_UnknownID_Errors verifies an unknown id fails before any change.
func TestE2E_Unlink_UnknownID_Errors(t *testing.T) {
	env, srcA, dstA, _, _ := unlinkFixture(t)
	mustSymlink(t, srcA, dstA)

	rootCmd.SetArgs([]string{"unlink", "--force", "1", "9"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no entry with id 9") {
		t.Fatalf("expected 'no entry with id 9' error, got %v", err)
	}
	env.AssertSymlink(t, ".a", srcA)

	rootCmd.SetArgs([]string{"unlink", "--force", "x"})
	err = rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid id") {
		t.Fatalf("expected 'invalid id' error, got %v", err)
	}
	env.AssertSymlink(t, ".a", srcA)
}

// TestE2E_Unlink_FailureExitsNonZero verifies a failed removal returns an error
// while the remaining entries are still processed.
func TestE2E_Unlink_FailureExitsNonZero(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	env.WriteHome(map[string]string{"ro/.keep": ""})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)

	srcA, srcB := filepath.Join(env.RepoDir, "files/.a"), filepath.Join(env.RepoDir, "files/.b")
	roDir := filepath.Join(env.HomeDir, "ro")
	dstA, dstB := filepath.Join(roDir, ".a"), filepath.Join(env.HomeDir, ".b")
	env.WriteConfig([]config.Entry{{ID: 1, Src: srcA, Dst: dstA}, {ID: 2, Src: srcB, Dst: dstB}})
	mustSymlink(t, srcA, dstA)
	mustSymlink(t, srcB, dstB)
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	rootCmd.SetArgs([]string{"unlink", "--force"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when unlink fails")
	}
	if _, err := os.Lstat(dstA); err != nil {
		t.Errorf("symlink in read-only dir should still exist: %v", err)
	}
	env.AssertNotExist(t, ".b")
}

// editDstEnv sets up a one-entry config (src, oldDst) and an $EDITOR that
// rewrites the entry to (src, newDst). It returns src, oldDst and newDst.
func editDstEnv(t *testing.T, env *testenv.Env) (src, oldDst, newDst string) {
	t.Helper()
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	src = filepath.Join(env.RepoDir, "files/.zshrc")
	oldDst = filepath.Join(env.HomeDir, ".zshrc")
	newDst = filepath.Join(env.HomeDir, ".zshrc2")
	env.WriteConfig([]config.Entry{{ID: 1, Src: src, Dst: oldDst}})

	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	body := fmt.Sprintf("#!/bin/sh\nprintf 'id: 1\\nsrc: %s\\ndst: %s\\n' > \"$1\"\n", src, newDst)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)
	return src, oldDst, newDst
}

// TestE2E_Edit_DstChange_UnlinksOldSymlink verifies that changing dst removes
// the old dst when it is a symlink to the entry's old src.
func TestE2E_Edit_DstChange_UnlinksOldSymlink(t *testing.T) {
	env := testenv.New(t)
	src, oldDst, newDst := editDstEnv(t, env)
	env.MakeSymlink(src, ".zshrc")

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("edit: %v", err)
	}

	env.AssertNotExist(t, ".zshrc")
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Entries[0].Dst != newDst {
		t.Errorf("Dst = %q, want %q (old %q)", cfg.Entries[0].Dst, newDst, oldDst)
	}
}

// TestE2E_Edit_DstChange_LeavesRealFile verifies a real file at the old dst
// is never removed.
func TestE2E_Edit_DstChange_LeavesRealFile(t *testing.T) {
	env := testenv.New(t)
	editDstEnv(t, env)
	env.WriteHome(map[string]string{".zshrc": "# mine\n"})

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("edit: %v", err)
	}

	env.AssertRegular(t, ".zshrc", "# mine\n")
}

// TestE2E_Edit_DstChange_LeavesForeignSymlink verifies a symlink at the old
// dst pointing somewhere other than the entry's src is left in place.
func TestE2E_Edit_DstChange_LeavesForeignSymlink(t *testing.T) {
	env := testenv.New(t)
	editDstEnv(t, env)
	other := filepath.Join(env.RepoDir, "files/other")
	env.MakeSymlink(other, ".zshrc")

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("edit: %v", err)
	}

	env.AssertSymlink(t, ".zshrc", other)
}

// TestE2E_Edit_EchoesUpdatedEntry verifies edit prints "updated <id>: <src> -> <dst>".
func TestE2E_Edit_EchoesUpdatedEntry(t *testing.T) {
	env := testenv.New(t)
	src, _, newDst := editDstEnv(t, env)

	rootCmd.SetArgs([]string{"edit", "1"})
	var execErr error
	out := captureStdout(func() { execErr = rootCmd.Execute() })
	if execErr != nil {
		t.Fatalf("edit: %v", execErr)
	}

	if want := fmt.Sprintf("updated 1: %s -> %s", src, newDst); !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
}

// TestE2E_Edit_SrcOnlyChange_LeavesDst verifies that changing only src touches
// nothing on disk: the symlink at dst still points at the old src.
func TestE2E_Edit_SrcOnlyChange_LeavesDst(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n", "files/.zshrc2": "# zsh2\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	oldSrc := filepath.Join(env.RepoDir, "files/.zshrc")
	newSrc := filepath.Join(env.RepoDir, "files/.zshrc2")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	env.WriteConfig([]config.Entry{{ID: 1, Src: oldSrc, Dst: dst}})
	env.MakeSymlink(oldSrc, ".zshrc")

	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	body := fmt.Sprintf("#!/bin/sh\nprintf 'id: 1\\nsrc: %s\\ndst: %s\\n' > \"$1\"\n", newSrc, dst)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("edit: %v", err)
	}

	env.AssertSymlink(t, ".zshrc", oldSrc)
}

// A repo path whose parent is a symlink could redirect the adopt move into a
// directory the repo owner does not control.
func TestE2E_Add_Adopt_SymlinkedSrcParent_Refused(t *testing.T) {
	env := testenv.New(t)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	other := filepath.Join(filepath.Dir(env.RepoDir), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(env.RepoDir, "ssh")); err != nil {
		t.Fatal(err)
	}
	env.WriteHome(map[string]string{".ssh/config": "secret\n"})

	src := filepath.Join(env.RepoDir, "ssh", "config")
	rootCmd.SetArgs([]string{"add", "--adopt", src, "~/.ssh/config"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected add --adopt to refuse a src whose parent is a symlink")
	}
	env.AssertRegular(t, ".ssh/config", "secret\n")
	if _, err := os.Lstat(filepath.Join(other, "config")); !os.IsNotExist(err) {
		t.Errorf("other/config should not exist, lstat err = %v", err)
	}
}

func homeDstSetup(t *testing.T, stdin string) *testenv.Env {
	t.Helper()
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"dir/f": "x\n"})
	env.WriteHome(map[string]string{"keep": "keep\n"})
	env.WriteConfig(nil)
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	prompt.Stdin = strings.NewReader(stdin)
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	return env
}

func TestE2E_Add_Adopt_Home_DefaultNo(t *testing.T) {
	env := homeDstSetup(t, "\n")
	copyDir := filepath.Join(env.RepoDir, "homecopy")

	rootCmd.SetArgs([]string{"add", "--adopt", copyDir, "~"})
	var execErr error
	captureStdout(func() { execErr = rootCmd.Execute() })
	if execErr != nil {
		t.Fatalf("add --adopt: %v", execErr)
	}
	env.AssertRegular(t, "keep", "keep\n")
	if _, err := os.Lstat(copyDir); err == nil {
		t.Errorf("%s exists but should not", copyDir)
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != 0 {
		t.Errorf("entries = %+v, want none", cfg.Entries)
	}
}

func TestE2E_Add_Home_Yes(t *testing.T) {
	env := homeDstSetup(t, "y\n")

	rootCmd.SetArgs([]string{"add", filepath.Join(env.RepoDir, "dir"), "~"})
	var execErr error
	captureStdout(func() { execErr = rootCmd.Execute() })
	if execErr != nil {
		t.Fatalf("add: %v", execErr)
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != 1 || cfg.Entries[0].Dst != env.HomeDir {
		t.Errorf("entries = %+v, want one with dst %s", cfg.Entries, env.HomeDir)
	}
}

func TestE2E_Add_Home_NoInput_Errors(t *testing.T) {
	env := homeDstSetup(t, "")

	rootCmd.SetArgs([]string{"add", filepath.Join(env.RepoDir, "dir"), "~"})
	var execErr error
	captureStdout(func() { execErr = rootCmd.Execute() })
	if execErr == nil || !strings.Contains(execErr.Error(), "--force") {
		t.Fatalf("want an error mentioning --force, got %v", execErr)
	}
}

func TestE2E_Add_Home_Force_Warns(t *testing.T) {
	env := homeDstSetup(t, "")

	rootCmd.SetArgs([]string{"add", "--force", filepath.Join(env.RepoDir, "dir"), "~"})
	var execErr error
	var stderr string
	captureStdout(func() {
		stderr = captureStderr(func() { execErr = rootCmd.Execute() })
	})
	if execErr != nil {
		t.Fatalf("add --force: %v", execErr)
	}
	if !strings.Contains(stderr, "warning:") {
		t.Errorf("stderr = %q, want a warning: line", stderr)
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != 1 || cfg.Entries[0].Dst != env.HomeDir {
		t.Errorf("entries = %+v, want one with dst %s", cfg.Entries, env.HomeDir)
	}
}

func TestE2E_Apply_HomeDst_BareEnter_No(t *testing.T) {
	env := homeDstSetup(t, "\n")
	env.WriteConfig([]config.Entry{{ID: 1, Src: filepath.Join(env.RepoDir, "dir"), Dst: env.HomeDir}})

	rootCmd.SetArgs([]string{"apply"})
	captureStdout(func() { _ = rootCmd.Execute() })

	env.AssertRegular(t, "keep", "keep\n")
	if _, err := os.Lstat(env.HomeDir + ".bk"); err == nil {
		t.Errorf("%s.bk exists but home must not be renamed", env.HomeDir)
	}
}

func TestE2E_Edit_HomeDst_DefaultNo(t *testing.T) {
	env := homeDstSetup(t, "\n")
	src := filepath.Join(env.RepoDir, "dir")
	oldDst := filepath.Join(env.HomeDir, ".d")
	env.WriteConfig([]config.Entry{{ID: 1, Src: src, Dst: oldDst}})
	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	body := fmt.Sprintf("#!/bin/sh\nprintf 'id: 1\\nsrc: %s\\ndst: \"%s\"\\n' > \"$1\"\n", src, env.HomeDir)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)

	rootCmd.SetArgs([]string{"edit", "1"})
	var execErr error
	captureStdout(func() { execErr = rootCmd.Execute() })
	if execErr != nil {
		t.Fatalf("edit: %v", execErr)
	}
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != 1 || cfg.Entries[0].Dst != oldDst {
		t.Errorf("entries = %+v, want dst unchanged %s", cfg.Entries, oldDst)
	}
}

// editToDst sets up an entry (repo file1 -> ~/.x) and an $EDITOR that rewrites
// its dst to newDst, with an existing ~/some_dir. It returns the sandbox env.
func editToDst(t *testing.T, newDst func(home string) string) *testenv.Env {
	t.Helper()
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"file1": "one\n"})
	env.WriteHome(map[string]string{"some_dir/other": "keep\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	src := filepath.Join(env.RepoDir, "file1")
	env.WriteConfig([]config.Entry{{ID: 1, Src: src, Dst: filepath.Join(env.HomeDir, ".x")}})

	script := filepath.Join(filepath.Dir(env.HomeDir), "editor.sh")
	body := fmt.Sprintf("#!/bin/sh\nprintf 'id: 1\\nsrc: %s\\ndst: %s\\n' > \"$1\"\n", src, newDst(env.HomeDir))
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)
	return env
}

func editedDst(t *testing.T, env *testenv.Env) string {
	t.Helper()
	cfg, err := config.Load(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != 1 {
		t.Fatalf("entries = %+v, want 1", cfg.Entries)
	}
	return cfg.Entries[0].Dst
}

// edit applies the same dst rules as add: a trailing / links inside the dir.
func TestE2E_Edit_TrailingSlashDst_LinksInside(t *testing.T) {
	env := editToDst(t, func(home string) string { return home + "/some_dir/" })

	rootCmd.SetArgs([]string{"edit", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got, want := editedDst(t, env), filepath.Join(env.HomeDir, "some_dir/file1"); got != want {
		t.Errorf("Dst = %q, want %q", got, want)
	}
}

// edit asks into/replace/cancel for a file src over an existing real directory.
func TestE2E_Edit_FileOverExistingDir_Prompts(t *testing.T) {
	for _, tc := range []struct{ answer, wantDst string }{
		{"\n", "some_dir/file1"},
		{"r\n", "some_dir"},
		{"c\n", ".x"},
	} {
		env := editToDst(t, func(home string) string { return home + "/some_dir" })
		prompt.Stdin = strings.NewReader(tc.answer)
		t.Cleanup(func() { prompt.Stdin = os.Stdin })

		rootCmd.SetArgs([]string{"edit", "1"})
		var execErr error
		captureStdout(func() { execErr = rootCmd.Execute() })
		if execErr != nil {
			t.Fatalf("answer %q: edit: %v", tc.answer, execErr)
		}
		if got, want := editedDst(t, env), filepath.Join(env.HomeDir, tc.wantDst); got != want {
			t.Errorf("answer %q: Dst = %q, want %q", tc.answer, got, want)
		}
	}
}

func TestE2E_Edit_FileOverExistingDir_NoInput_Errors(t *testing.T) {
	env := editToDst(t, func(home string) string { return home + "/some_dir" })
	prompt.Stdin = strings.NewReader("")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	rootCmd.SetArgs([]string{"edit", "1"})
	var execErr error
	captureStdout(func() { execErr = rootCmd.Execute() })
	if execErr == nil || !strings.Contains(execErr.Error(), "end dst with /") {
		t.Fatalf("want an error suggesting a trailing slash, got %v", execErr)
	}
	if got, want := editedDst(t, env), filepath.Join(env.HomeDir, ".x"); got != want {
		t.Errorf("config changed: Dst = %q, want %q", got, want)
	}
}

// mutatingReader runs mutate on its first Read, then yields "y\n": it models
// the filesystem changing while apply waits at the prompt.
type mutatingReader struct {
	mutate func()
	r      *strings.Reader
}

func (m *mutatingReader) Read(p []byte) (int, error) {
	if m.r == nil {
		m.mutate()
		m.r = strings.NewReader("y\n")
	}
	return m.r.Read(p)
}

// TestE2E_Apply_RelinkDstSwappedDuringPrompt verifies a symlink replaced by a
// user's regular file while the prompt is open is neither removed nor backed up.
func TestE2E_Apply_RelinkDstSwappedDuringPrompt(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n", "old/.zshrc": "# old\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	if err := os.Symlink(filepath.Join(env.RepoDir, "old/.zshrc"), dst); err != nil {
		t.Fatal(err)
	}
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	prompt.Stdin = &mutatingReader{mutate: func() {
		_ = os.Remove(dst)
		_ = os.WriteFile(dst, []byte("user data"), 0o644)
	}}
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	var err error
	stderr := captureStderr(func() {
		captureStdout(func() {
			rootCmd.SetArgs([]string{"apply"})
			err = rootCmd.Execute()
		})
	})
	env.AssertRegular(t, ".zshrc", "user data")
	env.AssertNotExist(t, ".zshrc"+env.BackupSuffix)
	if err == nil {
		t.Error("apply must fail when dst changed during the prompt")
	}
	if !strings.Contains(stderr, "re-run") {
		t.Errorf("stderr should tell the user to re-run apply, got: %q", stderr)
	}
}

// TestE2E_Apply_ConflictBackupAppearsDuringPrompt verifies a backup created
// while the prompt is open is not overwritten.
func TestE2E_Apply_ConflictBackupAppearsDuringPrompt(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	t.Setenv("DOTSYNC_CONFIG", env.ConfigPath)
	src := filepath.Join(env.RepoDir, "files/.zshrc")
	dst := filepath.Join(env.HomeDir, ".zshrc")
	rootCmd.SetArgs([]string{"add", "--src", src, "--dest", dst})
	_ = rootCmd.Execute()

	prompt.Stdin = &mutatingReader{mutate: func() {
		_ = os.WriteFile(dst+env.BackupSuffix, []byte("precious"), 0o644)
	}}
	t.Cleanup(func() { prompt.Stdin = os.Stdin })

	var err error
	stderr := captureStderr(func() {
		captureStdout(func() {
			rootCmd.SetArgs([]string{"apply"})
			err = rootCmd.Execute()
		})
	})
	env.AssertBackup(t, ".zshrc", "precious")
	if err == nil {
		t.Error("apply must fail when a backup appeared during the prompt")
	}
	if !strings.Contains(stderr, "re-run") {
		t.Errorf("stderr should tell the user to re-run apply, got: %q", stderr)
	}
}
