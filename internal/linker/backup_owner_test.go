package linker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/linker"
	"github.com/icadariu/dotSync/internal/testenv"
)

// Assumption: stubbing linker.Getuid makes every file look foreign, so this
// cannot isolate the backup from dst itself. It only asserts the observable
// contract: when a backup path exists and is not owned by the current uid,
// Apply (even with Force) errors and leaves that backup untouched.
func TestApply_ForeignBackup_Refused(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.vimrc": "new\n"})
	env.WriteHome(map[string]string{".vimrc": "cur\n", ".vimrc.bk": "theirs\n"})
	e := entry(env, "files/.vimrc", ".vimrc")

	old := linker.Getuid
	linker.Getuid = func() int { return os.Getuid() + 1 }
	t.Cleanup(func() { linker.Getuid = old })

	var err error
	captureOutput(func() {
		err = linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true})
	})
	if err == nil {
		t.Fatal("expected an error for a backup owned by another uid")
	}
	env.AssertRegular(t, ".vimrc.bk", "theirs\n")
	if fi, lerr := os.Lstat(filepath.Join(env.HomeDir, ".vimrc")); lerr != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf(".vimrc must stay a regular file, got %v (err %v)", fi, lerr)
	}
}
