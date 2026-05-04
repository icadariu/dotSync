package linker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/icadariu/dotsync/internal/config"
	"github.com/icadariu/dotsync/internal/linker"
	"github.com/icadariu/dotsync/internal/prompt"
	"github.com/icadariu/dotsync/internal/testenv"
)

func entry(env *testenv.Env, srcRel, dstRel string) config.Entry {
	return config.Entry{
		ID:  1,
		Src: filepath.Join(env.RepoDir, srcRel),
		Dst: filepath.Join(env.HomeDir, dstRel),
	}
}

// ---- SafeBackupPath ----

func TestSafeBackupPath_NoConflict(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "file.txt")
	got := linker.SafeBackupPath(dst, ".bk")
	if got != dst+".bk" {
		t.Errorf("got %q, want %q", got, dst+".bk")
	}
}

func TestSafeBackupPath_ConflictIncrementsToOne(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(dst+".bk", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := linker.SafeBackupPath(dst, ".bk")
	if got != dst+".bk.1" {
		t.Errorf("got %q, want %q", got, dst+".bk.1")
	}
}

func TestSafeBackupPath_MultipleConflicts(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "file.txt")
	for _, name := range []string{dst + ".bk", dst + ".bk.1", dst + ".bk.2"} {
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := linker.SafeBackupPath(dst, ".bk")
	if got != dst+".bk.3" {
		t.Errorf("got %q, want %q", got, dst+".bk.3")
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
	env.AssertRegular(t, ".zshrc.bk", "# previous backup\n")
	env.AssertRegular(t, ".zshrc.bk.1", "# original\n")
}

func TestApply_MissingSrc_ContinuesNext(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.present": "content\n"})
	missing := config.Entry{ID: 1, Src: filepath.Join(env.RepoDir, "missing"), Dst: filepath.Join(env.HomeDir, ".missing")}
	present := config.Entry{ID: 2, Src: filepath.Join(env.RepoDir, "files/.present"), Dst: filepath.Join(env.HomeDir, ".present")}
	if err := linker.Apply([]config.Entry{missing, present}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err != nil {
		t.Fatal(err)
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
