package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/icadariu/dotSync/internal/config"
	"github.com/icadariu/dotSync/internal/testenv"
)

// stubIdentity replaces the geteuid/fileOwner seams for one test.
func stubIdentity(t *testing.T, euid, owner int, ownerOK bool) {
	t.Helper()
	oldEuid, oldOwner := geteuid, fileOwner
	t.Cleanup(func() { geteuid, fileOwner = oldEuid, oldOwner })
	geteuid = func() int { return euid }
	fileOwner = func(string) (int, bool) { return owner, ownerOK }
}

// rootEnv returns a sandbox with a one-entry config and a src file.
func rootEnv(t *testing.T) (env *testenv.Env, src, dst string) {
	t.Helper()
	env = testenv.New(t)
	env.WriteRepo(map[string]string{"files/.rootguard": "x\n"})
	src = filepath.Join(env.RepoDir, "files/.rootguard")
	dst = filepath.Join(env.HomeDir, ".rootguard")
	env.WriteConfig([]config.Entry{{ID: 1, Src: src, Dst: dst}})
	return env, src, dst
}

func TestE2E_Root_UserOwnedConfig_Refused(t *testing.T) {
	env, _, _ := rootEnv(t)
	stubIdentity(t, 0, 1000, true)

	var err error
	out := captureStdout(func() {
		rootCmd.SetArgs([]string{"list"})
		err = rootCmd.Execute()
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to run as root") {
		t.Fatalf("err = %v, want 'refusing to run as root'", err)
	}
	if !strings.Contains(err.Error(), env.ConfigPath) || !strings.Contains(err.Error(), "1000") {
		t.Errorf("err must name config path and owner uid: %v", err)
	}
	if strings.Contains(out, ".rootguard") {
		t.Errorf("stdout must not list entries:\n%s", out)
	}
}

func TestE2E_Root_UserOwnedConfig_ForceStillRefused(t *testing.T) {
	env, _, _ := rootEnv(t)
	stubIdentity(t, 0, 1000, true)

	rootCmd.SetArgs([]string{"apply", "--force"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "refusing to run as root") {
		t.Fatalf("err = %v, want 'refusing to run as root'", err)
	}
	env.AssertNotExist(t, ".rootguard")
}

func TestE2E_Root_RootOwnedConfig_Allowed(t *testing.T) {
	rootEnv(t)
	stubIdentity(t, 0, 0, true)

	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
}

func TestE2E_Root_NoConfig_Allowed(t *testing.T) {
	env := testenv.New(t)
	env.WriteRepo(map[string]string{"files/.rootguard": "x\n"})
	if _, err := os.Stat(env.ConfigPath); err == nil {
		t.Fatal("config unexpectedly exists")
	}
	stubIdentity(t, 0, 0, false)

	rootCmd.SetArgs([]string{"add",
		filepath.Join(env.RepoDir, "files/.rootguard"),
		filepath.Join(env.HomeDir, ".rootguard")})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
}

func TestE2E_NonRoot_Unaffected(t *testing.T) {
	rootEnv(t)
	stubIdentity(t, 1000, 0, true)

	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
}

func TestE2E_Root_CompletionExempt(t *testing.T) {
	rootEnv(t)
	stubIdentity(t, 0, 1000, true)

	var err error
	captureStdout(func() {
		rootCmd.SetArgs([]string{"completion", "bash"})
		err = rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("completion bash: %v", err)
	}
}
