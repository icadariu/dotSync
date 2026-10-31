package linker_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/linker"
	"github.com/icadariu/dotSync/internal/prompt"
	"github.com/icadariu/dotSync/internal/testenv"
)

func entry(env *testenv.Env, srcRel, dstRel string) config.Entry {
	return config.Entry{
		ID:  1,
		Src: filepath.Join(env.RepoDir, srcRel),
		Dst: filepath.Join(env.HomeDir, dstRel),
	}
}

// ---- Plan ----

func TestPlan_FreshLink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	results := linker.Plan([]config.Entry{e})
	if len(results) != 1 || results[0].Status != linker.StatusLink {
		t.Errorf("expected StatusLink, got %v", results)
	}
}

func TestPlan_Unchanged(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	src := filepath.Join(env.RepoDir, "files/.zshrc")
	env.MakeSymlink(src, ".zshrc")
	e := entry(env, "files/.zshrc", ".zshrc")
	results := linker.Plan([]config.Entry{e})
	if len(results) != 1 || results[0].Status != linker.StatusOK {
		t.Errorf("expected StatusOK, got %v", results)
	}
}

func TestPlan_Conflict_RealFile(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	results := linker.Plan([]config.Entry{e})
	if len(results) != 1 || results[0].Status != linker.StatusConflict {
		t.Errorf("expected StatusConflict, got %v", results)
	}
	if !strings.Contains(results[0].Message, "-# old") {
		t.Errorf("expected removal in diff, got:\n%s", results[0].Message)
	}
	if !strings.Contains(results[0].Message, "+# new") {
		t.Errorf("expected addition in diff, got:\n%s", results[0].Message)
	}
}

func TestPlan_MissingSrc(t *testing.T) {
	env := testenv.New(t)
	e := config.Entry{ID: 1, Src: filepath.Join(env.RepoDir, "missing"), Dst: filepath.Join(env.HomeDir, ".x")}
	results := linker.Plan([]config.Entry{e})
	if len(results) != 1 || results[0].Status != linker.StatusError {
		t.Errorf("expected StatusError, got %v", results)
	}
}

func TestPlan_SrcEqualsDst_Error(t *testing.T) {
	env := testenv.New(t)
	env.WriteHome(map[string]string{".gitconfig": "precious\n"})
	p := filepath.Join(env.HomeDir, ".gitconfig")
	results := linker.Plan([]config.Entry{{ID: 1, Src: p, Dst: p}})
	if len(results) != 1 || results[0].Status != linker.StatusError {
		t.Errorf("expected StatusError, got %v", results)
	}
}

func TestApply_SrcEqualsDst_NoBackup_KeepsFile(t *testing.T) {
	env := testenv.New(t)
	env.WriteHome(map[string]string{".gitconfig": "precious\n"})
	p := filepath.Join(env.HomeDir, ".gitconfig")
	e := config.Entry{ID: 1, Src: p, Dst: p}
	_ = linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true, NoBackup: true})
	env.AssertRegular(t, ".gitconfig", "precious\n")
}

// ---- Apply ----

func TestApply_FreshLink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
}

func TestApply_Idempotent(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	opts := linker.ApplyOptions{BackupSuffix: ".bk", Force: true}
	if err := linker.Apply([]config.Entry{e}, opts); err != nil {
		t.Fatal(err)
	}
	if err := linker.Apply([]config.Entry{e}, opts); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
	env.AssertNotExist(t, ".zshrc.bk")
}

func TestApply_ConflictRealFile_Backup(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
	env.AssertBackup(t, ".zshrc", "# old\n")
}

// A second backup replaces the first: only dst+suffix is ever kept.
func TestApply_ConflictBackupCollision(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{
		".zshrc":    "# original\n",
		".zshrc.bk": "# previous backup\n",
	})
	e := entry(env, "files/.zshrc", ".zshrc")
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
	env.AssertRegular(t, ".zshrc.bk", "# original\n")
	env.AssertNotExist(t, ".zshrc.bk.1")
}

func TestApply_Relink_NoBackup(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n", "files/.other": "# other\n"})
	env.MakeSymlink(filepath.Join(env.RepoDir, "files/.other"), ".zshrc")
	e := entry(env, "files/.zshrc", ".zshrc")
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
	env.AssertNotExist(t, ".zshrc.bk")
}

// Re-pointing a managed link must not clobber the backup of the user's
// original file.
func TestApply_OriginalPreservedAcrossRelink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"a": "A\n", "b": "B\n"})
	env.WriteHome(map[string]string{".vimrc": "original\n"})
	opts := linker.ApplyOptions{BackupSuffix: ".bk", Force: true}
	if err := linker.Apply([]config.Entry{entry(env, "a", ".vimrc")}, opts); err != nil {
		t.Fatal(err)
	}
	env.AssertBackup(t, ".vimrc", "original\n")
	if err := linker.Apply([]config.Entry{entry(env, "b", ".vimrc")}, opts); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".vimrc", filepath.Join(env.RepoDir, "b"))
	env.AssertBackup(t, ".vimrc", "original\n")
}

func existingBackupEntry(t *testing.T) (*testenv.Env, config.Entry) {
	t.Helper()
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.vimrc": "new\n"})
	env.WriteHome(map[string]string{".vimrc": "cur\n", ".vimrc.bk": "old\n"})
	return env, entry(env, "files/.vimrc", ".vimrc")
}

// The backup must be a sibling of dst: a suffix that climbs out of the
// directory would otherwise make the overwrite delete an unrelated path.
func TestApply_BackupOutsideDstDir_Refused(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/cfg/x": "new\n"})
	env.WriteHome(map[string]string{".cfg/x": "cur\n", "victim/keep": "keep\n"})
	e := entry(env, "files/cfg", ".cfg")
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: "/../victim", Force: true}); err == nil {
		t.Error("want an error for a backup path outside dst's directory")
	}
	env.AssertRegular(t, "victim/keep", "keep\n")
	env.AssertRegular(t, ".cfg/x", "cur\n")
}

func TestPlan_ExistingBackup_WillBeOverwritten(t *testing.T) {
	env, e := existingBackupEntry(t)
	results := linker.Plan([]config.Entry{e})
	if len(results) != 1 || results[0].Status != linker.StatusConflict {
		t.Fatalf("expected StatusConflict, got %v", results)
	}
	want := "backup " + filepath.Join(env.HomeDir, ".vimrc.bk") + " will be overwritten"
	if !strings.Contains(results[0].Message, want) {
		t.Errorf("message should contain %q, got:\n%s", want, results[0].Message)
	}
}

func TestApply_ExistingBackup_Force_OverwritesAndWarns(t *testing.T) {
	env, e := existingBackupEntry(t)
	var err error
	_, errOut := captureOutput(func() {
		err = linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".vimrc", filepath.Join(env.RepoDir, "files/.vimrc"))
	env.AssertBackup(t, ".vimrc", "cur\n")
	if !strings.Contains(errOut, "warning: overwrote backup") {
		t.Errorf("stderr should warn about the overwrite, got %q", errOut)
	}
}

func TestApply_ExistingBackupDir_Force_Replaced(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.cfg/a": "new\n"})
	env.WriteHome(map[string]string{".cfg/x": "new-ish\n", ".cfg.bk/y": "stale\n"})
	e := entry(env, "files/.cfg", ".cfg")
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".cfg", filepath.Join(env.RepoDir, "files/.cfg"))
	env.AssertRegular(t, ".cfg.bk/x", "new-ish\n")
	env.AssertNotExist(t, ".cfg.bk/y")
}

func TestApply_MissingSrc_ContinuesNext(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.present": "content\n"})
	missing := config.Entry{ID: 1, Src: filepath.Join(env.RepoDir, "missing"), Dst: filepath.Join(env.HomeDir, ".missing")}
	present := config.Entry{ID: 2, Src: filepath.Join(env.RepoDir, "files/.present"), Dst: filepath.Join(env.HomeDir, ".present")}
	if err := linker.Apply([]config.Entry{missing, present}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err == nil {
		t.Fatal("expected error for missing src")
	}
	env.AssertNotExist(t, ".missing")
	env.AssertSymlink(t, ".present", filepath.Join(env.RepoDir, "files/.present"))
}

func TestApply_Force_SkipsPrompt(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	prompt.Stdin = strings.NewReader("")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
}

func TestApply_NoForce_PromptAccept(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	prompt.Stdin = strings.NewReader("y\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: false}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
}

func TestApply_NoForce_PromptDecline(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	prompt.Stdin = strings.NewReader("n\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: false}); err != nil {
		t.Fatal(err)
	}
	env.AssertNotExist(t, ".zshrc")
}

// TestApply_NoForce_PipedAnswerPerEntry verifies each entry's prompt reads its
// own line of piped input.
func TestApply_NoForce_PipedAnswerPerEntry(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	a := entry(env, "files/.a", ".a")
	b := entry(env, "files/.b", ".b")
	b.ID = 2
	prompt.Stdin = strings.NewReader("y\nn\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{a, b}, linker.ApplyOptions{BackupSuffix: ".bk"}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".a", filepath.Join(env.RepoDir, "files/.a"))
	env.AssertNotExist(t, ".b")
}

// TestApply_NoForce_NoInput_Aborts verifies apply without --force and without
// any stdin errors out instead of auto-confirming, leaving a real dst intact
// even with NoBackup.
func TestApply_NoForce_NoInput_Aborts(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# zsh\n"})
	env.WriteHome(map[string]string{".zshrc": "precious\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	prompt.Stdin = strings.NewReader("")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", NoBackup: true}); err == nil {
		t.Fatal("expected an error when there is no input")
	}
	env.AssertRegular(t, ".zshrc", "precious\n")
}

func TestPlan_Relink_WrongSymlink(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{
		"files/.zshrc": "# new\n",
		"files/.other": "# other\n",
	})
	// dst is a symlink but points to the wrong target
	env.MakeSymlink(filepath.Join(env.RepoDir, "files/.other"), ".zshrc")
	e := entry(env, "files/.zshrc", ".zshrc")
	results := linker.Plan([]config.Entry{e})
	if len(results) != 1 || results[0].Status != linker.StatusRelink {
		t.Errorf("expected StatusRelink for wrong symlink, got %v", results)
	}
}

func TestPlan_Conflict_TypeMismatch_DirSrcFileDst(t *testing.T) {
	env := testenv.New(t)
	// src is a directory
	if err := os.MkdirAll(filepath.Join(env.RepoDir, "dirs/kitty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.RepoDir, "dirs/kitty/kitty.conf"), []byte("config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// dst is a regular file
	dst := filepath.Join(env.HomeDir, ".config", "kitty")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := config.Entry{
		ID:  1,
		Src: filepath.Join(env.RepoDir, "dirs/kitty"),
		Dst: dst,
	}
	results := linker.Plan([]config.Entry{e})
	if len(results) != 1 || results[0].Status != linker.StatusConflict {
		t.Errorf("expected StatusConflict for dir/file type mismatch, got %v", results)
	}
}

func TestApply_NoForce_PromptAll(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	a := entry(env, "files/.a", ".a")
	b := entry(env, "files/.b", ".b")
	b.ID = 2
	prompt.Stdin = strings.NewReader("a\nn\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{a, b}, linker.ApplyOptions{BackupSuffix: ".bk", Force: false}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".a", filepath.Join(env.RepoDir, "files/.a"))
	env.AssertSymlink(t, ".b", filepath.Join(env.RepoDir, "files/.b"))
}

func TestApply_NoForce_PromptQuit(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	a := entry(env, "files/.a", ".a")
	b := entry(env, "files/.b", ".b")
	b.ID = 2
	prompt.Stdin = strings.NewReader("q\ny\ny\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{a, b}, linker.ApplyOptions{BackupSuffix: ".bk", Force: false}); err != nil {
		t.Fatal(err)
	}
	env.AssertNotExist(t, ".a")
	env.AssertNotExist(t, ".b")
}

// TestPlan_FIFOAtDst_DoesNotBlock verifies Plan does not open a FIFO at dst
// to diff it.
func TestPlan_FIFOAtDst_DoesNotBlock(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.f": "x\n"})
	if err := syscall.Mkfifo(filepath.Join(env.HomeDir, ".f"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := entry(env, "files/.f", ".f")

	done := make(chan struct{})
	go func() {
		defer close(done)
		linker.Plan([]config.Entry{e})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Plan blocked on a FIFO at dst")
	}
}

// TestPlan_DuplicateDst_LaterEntryIsError checks an entry repeating an earlier
// entry's dst is a StatusError and the first one stays a normal link.
func TestPlan_DuplicateDst_LaterEntryIsError(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	a := entry(env, "files/.a", ".x")
	b := entry(env, "files/.b", ".x")
	b.ID = 2

	results := linker.Plan([]config.Entry{a, b})
	if results[0].Status != linker.StatusLink {
		t.Errorf("results[0].Status = %v, want StatusLink", results[0].Status)
	}
	if results[1].Status != linker.StatusError {
		t.Errorf("results[1].Status = %v, want StatusError", results[1].Status)
	}
	if !strings.Contains(results[1].Message, "dst also used by entry 1") {
		t.Errorf("results[1].Message = %q, want it to contain %q", results[1].Message, "dst also used by entry 1")
	}
}

// TestApply_DuplicateDst_FirstWinsNoFlipFlop checks the first entry wins, the
// duplicate is skipped, Apply errors, and repeated runs do not relink or back up.
func TestApply_DuplicateDst_FirstWinsNoFlipFlop(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	a := entry(env, "files/.a", ".x")
	b := entry(env, "files/.b", ".x")
	b.ID = 2

	for i := 0; i < 2; i++ {
		err := linker.Apply([]config.Entry{a, b}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true})
		if err == nil {
			t.Errorf("Apply call %d: expected non-nil error for duplicate dst", i+1)
		}
	}
	env.AssertSymlink(t, ".x", filepath.Join(env.RepoDir, "files/.a"))
	env.AssertNotExist(t, ".x.bk")
}

// captureOutput runs f and returns what it wrote to os.Stdout and os.Stderr.
func captureOutput(f func()) (stdout, stderr string) {
	oldOut, oldErr := os.Stdout, os.Stderr
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	os.Stdout, os.Stderr = wo, we
	f()
	wo.Close()
	we.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	var so, se strings.Builder
	_, _ = io.Copy(&so, ro)
	_, _ = io.Copy(&se, re)
	return so.String(), se.String()
}

// Deleting a file without a backup is irreversible, so even --force says so on stderr.
func TestApply_NoBackup_WarnsOnDelete(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	var err error
	_, stderr := captureOutput(func() {
		err = linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true, NoBackup: true})
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := "warning: deleted " + filepath.Join(env.HomeDir, ".zshrc") + " (no backup)"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr, want)
	}
}

// With --no-backup a conflict destroys the user's file, so a bare Enter must decline.
func TestApply_NoBackup_Conflict_EnterDeclines(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	prompt.Stdin = strings.NewReader("\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	var err error
	out, _ := captureOutput(func() {
		err = linker.Apply([]config.Entry{e}, linker.ApplyOptions{NoBackup: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[y/N/a/q]") {
		t.Errorf("prompt hint [y/N/a/q] missing from stdout: %q", out)
	}
	env.AssertRegular(t, ".zshrc", "# old\n")
}

// A relink only drops a symlink, so --no-backup keeps the default-yes prompt.
func TestApply_NoBackup_Relink_EnterAccepts(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n", "files/.other": "# other\n"})
	env.MakeSymlink(filepath.Join(env.RepoDir, "files/.other"), ".zshrc")
	e := entry(env, "files/.zshrc", ".zshrc")
	prompt.Stdin = strings.NewReader("\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{NoBackup: true}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
}

// With backups on, Enter on a conflict still accepts (default stays Y).
func TestApply_Backup_Conflict_EnterAccepts(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	prompt.Stdin = strings.NewReader("\n")
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk"}); err != nil {
		t.Fatal(err)
	}
	env.AssertSymlink(t, ".zshrc", filepath.Join(env.RepoDir, "files/.zshrc"))
	env.AssertBackup(t, ".zshrc", "# old\n")
}

// A non-empty directory cannot be removed without backup: refuse clearly, keep it intact.
func TestApply_NoBackup_NonEmptyDir_RefusedClearly(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.cfg/a": "new\n"})
	env.WriteHome(map[string]string{".cfg/keep": "precious\n"})
	e := entry(env, "files/.cfg", ".cfg")
	var err error
	_, errOut := captureOutput(func() {
		err = linker.Apply([]config.Entry{e}, linker.ApplyOptions{NoBackup: true, Force: true})
	})
	if err == nil {
		t.Fatal("expected a non-nil error")
	}
	if !strings.Contains(errOut, "--no-backup") {
		t.Errorf("stderr should mention --no-backup: %q", errOut)
	}
	if n := strings.Count(errOut, e.Dst); n != 1 {
		t.Errorf("dst path appears %d times in stderr, want 1: %q", n, errOut)
	}
	env.AssertRegular(t, ".cfg/keep", "precious\n")
}

// ---- Apply stdout report ----

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

func TestApply_PrintsLinkedAndSummary(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	fresh := entry(env, "files/.a", ".a")
	done := config.Entry{ID: 2, Src: filepath.Join(env.RepoDir, "files/.b"), Dst: filepath.Join(env.HomeDir, ".b")}
	if err := os.Symlink(done.Src, done.Dst); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(func() {
		if err := linker.Apply([]config.Entry{fresh, done}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
			t.Fatal(err)
		}
	})
	want := "linked " + fresh.Dst + " -> " + fresh.Src + "\n1 linked, 1 unchanged\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestApply_PrintsBackupPath(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "# new\n"})
	env.WriteHome(map[string]string{".zshrc": "# old\n"})
	e := entry(env, "files/.zshrc", ".zshrc")
	out := captureStdout(func() {
		if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
			t.Fatal(err)
		}
	})
	want := "linked " + e.Dst + " -> " + e.Src + " (backup: " + e.Dst + ".bk)\n1 linked, 0 unchanged\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestApply_SummaryCountsFailed(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.present": "x\n"})
	missing := config.Entry{ID: 1, Src: filepath.Join(env.RepoDir, "missing"), Dst: filepath.Join(env.HomeDir, ".missing")}
	present := config.Entry{ID: 2, Src: filepath.Join(env.RepoDir, "files/.present"), Dst: filepath.Join(env.HomeDir, ".present")}
	out := captureStdout(func() {
		if err := linker.Apply([]config.Entry{missing, present}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err == nil {
			t.Fatal("expected error for missing src")
		}
	})
	if !strings.HasSuffix(out, "1 linked, 0 unchanged, 1 failed\n") {
		t.Errorf("stdout = %q, want summary suffix %q", out, "1 linked, 0 unchanged, 1 failed\n")
	}
}

// applyTwoFresh applies two fresh entries with the given prompt input and returns stdout.
func applyTwoFresh(t *testing.T, input string) string {
	t.Helper()
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.a": "a\n", "files/.b": "b\n"})
	a := entry(env, "files/.a", ".a")
	b := entry(env, "files/.b", ".b")
	b.ID = 2
	prompt.Stdin = strings.NewReader(input)
	t.Cleanup(func() { prompt.Stdin = os.Stdin })
	return captureStdout(func() {
		if err := linker.Apply([]config.Entry{a, b}, linker.ApplyOptions{BackupSuffix: ".bk"}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestApply_SummaryCountsSkipped(t *testing.T) {
	out := applyTwoFresh(t, "n\ny\n")
	if want := "1 linked, 0 unchanged, 1 skipped\n"; !strings.HasSuffix(out, want) {
		t.Errorf("stdout = %q, want suffix %q", out, want)
	}
}

func TestApply_QuitStillPrintsSummary(t *testing.T) {
	out := applyTwoFresh(t, "y\nq\n")
	if want := "1 linked, 0 unchanged\n"; !strings.HasSuffix(out, want) {
		t.Errorf("stdout = %q, want suffix %q", out, want)
	}
}

// ---- Plan: never read through symlinks, cap file size (#91) ----

func TestPlan_SrcSymlink_NotReadThrough(t *testing.T) {
	env := testenv.New(t)
	env.WriteHome(map[string]string{"secret.txt": "FAKE-SECRET\n"})
	if err := os.Symlink(filepath.Join(env.HomeDir, "secret.txt"), filepath.Join(env.RepoDir, "vimrc")); err != nil {
		t.Fatal(err)
	}
	results := linker.Plan([]config.Entry{entry(env, "vimrc", ".vimrc")})
	if len(results) != 1 || results[0].Status != linker.StatusLink {
		t.Fatalf("expected StatusLink, got %v", results)
	}
	if strings.Contains(results[0].Message, "FAKE-SECRET") {
		t.Errorf("message read through src symlink:\n%s", results[0].Message)
	}
}

func TestPlan_RelinkDoesNotReadOldTarget(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.zshrc": "new\n"})
	env.WriteHome(map[string]string{"secret.txt": "FAKE-SECRET\n"})
	env.MakeSymlink(filepath.Join(env.HomeDir, "secret.txt"), ".zshrc")
	results := linker.Plan([]config.Entry{entry(env, "files/.zshrc", ".zshrc")})
	if len(results) != 1 || results[0].Status != linker.StatusRelink {
		t.Fatalf("expected StatusRelink, got %v", results)
	}
	if strings.Contains(results[0].Message, "FAKE-SECRET") {
		t.Errorf("message read old symlink target:\n%s", results[0].Message)
	}
}

func TestPlan_DstSymlinkToDir_NotWalked(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"dirs/kitty/kitty.conf": "config\n"})
	env.WriteHome(map[string]string{"elsewhere/deep/x": "FAKE-SECRET-DEEP\n"})
	env.MakeSymlink(filepath.Join(env.HomeDir, "elsewhere"), ".kitty")
	results := linker.Plan([]config.Entry{entry(env, "dirs/kitty", ".kitty")})
	if len(results) != 1 || results[0].Status != linker.StatusRelink {
		t.Fatalf("expected StatusRelink, got %v", results)
	}
	if strings.Contains(results[0].Message, "FAKE-SECRET-DEEP") {
		t.Errorf("message walked dst symlinked dir:\n%s", results[0].Message)
	}
}

func TestPlan_LargeFile_NotDiffed(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"big": string(bytes.Repeat([]byte("a\n"), 1<<20))})
	results := linker.Plan([]config.Entry{entry(env, "big", "big")})
	if len(results) != 1 || results[0].Status != linker.StatusLink {
		t.Fatalf("expected StatusLink, got %v", results)
	}
	if want := "file too large to diff (2097152 bytes)"; !strings.Contains(results[0].Message, want) {
		t.Errorf("want %q in message, got:\n%.300s", want, results[0].Message)
	}
}

func TestPlan_LargeConflict_NotDiffed(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"f": "small\n"})
	env.WriteHome(map[string]string{"f": string(bytes.Repeat([]byte("a\n"), 1<<20))})
	results := linker.Plan([]config.Entry{entry(env, "f", "f")})
	if len(results) != 1 || results[0].Status != linker.StatusConflict {
		t.Fatalf("expected StatusConflict, got %v", results)
	}
	if want := "file too large to diff (2097152 bytes)"; !strings.Contains(results[0].Message, want) {
		t.Errorf("want %q in message, got:\n%.300s", want, results[0].Message)
	}
}

// ---- Untrusted paths (#90) ----

func mustChmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

// sharedRCSetup builds repo/bashrc -> <sandbox>/shared/rc where shared is
// world-writable without the sticky bit, and returns the entry for it.
func sharedRCSetup(t *testing.T) (*testenv.Env, config.Entry) {
	t.Helper()
	env := testenv.New(t)
	shared := filepath.Join(filepath.Dir(env.RepoDir), "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "rc"), []byte("# rc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustChmod(t, shared, 0o777)
	if err := os.Symlink(filepath.Join(shared, "rc"), filepath.Join(env.RepoDir, "bashrc")); err != nil {
		t.Fatal(err)
	}
	return env, entry(env, "bashrc", ".bashrc")
}

func TestPlan_SrcThroughSymlink_ShowsResolved(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"real/vimrc": "set nu\n"})
	if err := os.Symlink(filepath.Join(env.RepoDir, "real/vimrc"), filepath.Join(env.RepoDir, "vimrc")); err != nil {
		t.Fatal(err)
	}
	results := linker.Plan([]config.Entry{entry(env, "vimrc", ".vimrc")})
	if results[0].Status != linker.StatusLink {
		t.Fatalf("status = %v, want StatusLink (msg %q)", results[0].Status, results[0].Message)
	}
	want := "src resolves to " + filepath.Join(env.RepoDir, "real/vimrc")
	if !strings.Contains(results[0].Message, want) {
		t.Errorf("message %q does not contain %q", results[0].Message, want)
	}
}

func TestPlan_SrcResolvesIntoWorldWritableDir_Unsafe(t *testing.T) {
	_, e := sharedRCSetup(t)
	results := linker.Plan([]config.Entry{e})
	if results[0].Status != linker.StatusError || !strings.Contains(results[0].Message, "unsafe") {
		t.Errorf("want StatusError with \"unsafe\", got %v %q", results[0].Status, results[0].Message)
	}
}

func TestPlan_WorldWritableSrcFile_Unsafe(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/rc": "# rc\n"})
	mustChmod(t, filepath.Join(env.RepoDir, "files/rc"), 0o666)
	results := linker.Plan([]config.Entry{entry(env, "files/rc", ".rc")})
	if results[0].Status != linker.StatusError || !strings.Contains(results[0].Message, "unsafe") {
		t.Errorf("want StatusError with \"unsafe\", got %v %q", results[0].Status, results[0].Message)
	}
}

func TestPlan_GroupWritableSrc_Allowed(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/rc": "# rc\n"})
	mustChmod(t, filepath.Join(env.RepoDir, "files/rc"), 0o664)
	mustChmod(t, filepath.Join(env.RepoDir, "files"), 0o775)
	results := linker.Plan([]config.Entry{entry(env, "files/rc", ".rc")})
	if results[0].Status != linker.StatusLink {
		t.Errorf("want StatusLink, got %v %q", results[0].Status, results[0].Message)
	}
}

func dstParentSetup(t *testing.T, mode os.FileMode) []config.Entry {
	t.Helper()
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/rc": "# rc\n"})
	parent := filepath.Join(env.HomeDir, "sub")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	mustChmod(t, parent, mode)
	return []config.Entry{entry(env, "files/rc", "sub/.rc")}
}

func TestPlan_DstParentWorldWritable_Unsafe(t *testing.T) {
	results := linker.Plan(dstParentSetup(t, 0o777))
	if results[0].Status != linker.StatusError || !strings.Contains(results[0].Message, "unsafe") {
		t.Errorf("want StatusError with \"unsafe\", got %v %q", results[0].Status, results[0].Message)
	}
}

func TestPlan_DstParentSticky_Allowed(t *testing.T) {
	results := linker.Plan(dstParentSetup(t, 0o777|os.ModeSticky))
	if results[0].Status != linker.StatusLink {
		t.Errorf("want StatusLink, got %v %q", results[0].Status, results[0].Message)
	}
}

// A missing dst parent must not skip the trust check: the nearest existing
// ancestor is checked instead, since MkdirAll would build inside it.
func TestPlan_DstMissingParentUnderWorldWritable_Unsafe(t *testing.T) {
	for _, rel := range []string{"sub/missing/.rc", "sub/a/b/.rc"} {
		e := dstParentSetup(t, 0o777)
		e[0].Dst = filepath.Join(filepath.Dir(filepath.Dir(e[0].Dst)), rel)
		results := linker.Plan(e)
		if results[0].Status != linker.StatusError || !strings.Contains(results[0].Message, "unsafe dst parent") {
			t.Errorf("%s: want StatusError with \"unsafe dst parent\", got %v %q", rel, results[0].Status, results[0].Message)
		}
	}
}

func TestPlan_DstMissingParentUnderSafeOrSticky_Allowed(t *testing.T) {
	for _, mode := range []os.FileMode{0o755, 0o777 | os.ModeSticky} {
		e := dstParentSetup(t, mode)
		e[0].Dst = filepath.Join(filepath.Dir(e[0].Dst), "missing", ".rc")
		results := linker.Plan(e)
		if results[0].Status != linker.StatusLink {
			t.Errorf("mode %v: want StatusLink, got %v %q", mode, results[0].Status, results[0].Message)
		}
	}
}

func TestApply_UnsafeEntry_RefusedEvenWithForce(t *testing.T) {
	env, e := sharedRCSetup(t)
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err == nil {
		t.Fatal("expected error for unsafe entry even with Force")
	}
	env.AssertNotExist(t, ".bashrc")
}

// Missing dst parents take the mode of their nearest existing ancestor, so a
// directory created under a private ~/.config is private too.
func TestApply_CreatesMissingParentsWithAncestorMode(t *testing.T) {
	for _, mode := range []os.FileMode{0o700, 0o750} {
		env := testenv.New(t)
		env.WriteRepo(map[string]string{"files/init.lua": "x\n"})
		cfgDir := filepath.Join(env.HomeDir, ".config")
		if err := os.Mkdir(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(cfgDir, mode); err != nil {
			t.Fatal(err)
		}
		e := entry(env, "files/init.lua", ".config/nvim/lua/init.lua")
		if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		for _, d := range []string{".config/nvim", ".config/nvim/lua"} {
			fi, err := os.Stat(filepath.Join(env.HomeDir, d))
			if err != nil {
				t.Fatal(err)
			}
			if got := fi.Mode().Perm(); got != mode {
				t.Errorf("ancestor %o: %s mode = %o, want %o", mode, d, got, mode)
			}
		}
	}
}

// Inheriting may only narrow the old 0755: a world-writable ancestor (like a
// shared dir) must not make the directories apply creates writable by others.
// The shared dir is sticky, like /tmp: a non-sticky one is refused as unsafe.
func TestApply_CreatedParentsNeverGroupOrWorldWritable(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/rc": "x\n"})
	shared := filepath.Join(env.HomeDir, "shared")
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	e := entry(env, "files/rc", "shared/app/rc")
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	fi, err := os.Stat(filepath.Join(shared, "app"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		t.Errorf("shared/app mode = %o, want no group/world write", fi.Mode().Perm())
	}
}
