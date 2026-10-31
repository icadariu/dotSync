package linker_test

import (
	"os"
	"strings"
	"testing"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/linker"
	"github.com/icadariu/dotSync/internal/testenv"
)

func TestPlan_ForeignOwner_Unsafe(t *testing.T) {
	orig := linker.Getuid
	linker.Getuid = func() int { return os.Getuid() + 1 }
	t.Cleanup(func() { linker.Getuid = orig })

	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/rc": "# rc\n"})
	results := linker.Plan([]config.Entry{entry(env, "files/rc", ".rc")})
	if results[0].Status != linker.StatusError || !strings.Contains(results[0].Message, "unsafe") {
		t.Errorf("want StatusError with \"unsafe\", got %v %q", results[0].Status, results[0].Message)
	}
}

func TestApply_ForeignOwner_RefusedEvenWithForce(t *testing.T) {
	orig := linker.Getuid
	linker.Getuid = func() int { return os.Getuid() + 1 }
	t.Cleanup(func() { linker.Getuid = orig })

	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/rc": "# rc\n"})
	e := entry(env, "files/rc", ".rc")
	if err := linker.Apply([]config.Entry{e}, linker.ApplyOptions{BackupSuffix: ".bk", Force: true}); err == nil {
		t.Fatal("expected error for foreign-owned entry even with Force")
	}
	env.AssertNotExist(t, ".rc")
}
