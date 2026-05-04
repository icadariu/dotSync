package testenv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/icadariu/dotsync/internal/config"
	"gopkg.in/yaml.v3"
)

// TestRoot is where every test sandbox lives. The user's cron cleans this
// directory; tests must never read or write outside it.
const TestRoot = "/tmp/dotsync_tests"

// NextTestDir creates and returns /tmp/dotsync_tests/testN where N is the
// next unused integer (1, 2, 3, …). The directory is NOT cleaned up — the
// user's cron handles that.
func NextTestDir() string {
	if err := os.MkdirAll(TestRoot, 0o755); err != nil {
		panic(fmt.Errorf("creating %s: %w", TestRoot, err))
	}
	for i := 1; i < 1<<20; i++ {
		candidate := filepath.Join(TestRoot, fmt.Sprintf("test%d", i))
		err := os.Mkdir(candidate, 0o755)
		if err == nil {
			return candidate
		}
		if !os.IsExist(err) {
			panic(fmt.Errorf("creating %s: %w", candidate, err))
		}
	}
	panic("ran out of test directory slots under " + TestRoot)
}

// Env is a sandboxed filesystem environment for tests.
type Env struct {
	HomeDir      string
	RepoDir      string
	ConfigPath   string
	BackupSuffix string
	t            *testing.T
}

// New creates a fresh sandbox under /tmp/dotsync_tests/testN with isolated
// HomeDir, RepoDir, and ConfigPath. HOME and DOTSYNC_CONFIG are redirected
// so the binary never touches the user's real ~/.dotsync.yaml.
func New(t *testing.T) *Env {
	t.Helper()
	dir := NextTestDir()
	home := filepath.Join(dir, "home")
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("New MkdirAll home: %v", err)
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("New MkdirAll repo: %v", err)
	}
	cfgPath := filepath.Join(home, ".dotsync.yaml")
	t.Setenv("HOME", home)
	t.Setenv("DOTSYNC_CONFIG", cfgPath)
	return &Env{
		HomeDir:      home,
		RepoDir:      repo,
		ConfigPath:   cfgPath,
		BackupSuffix: ".bk",
		t:            t,
	}
}

// WriteRepo creates files inside RepoDir. Keys are relative paths, values are content.
func (e *Env) WriteRepo(files map[string]string) {
	e.t.Helper()
	for rel, content := range files {
		path := filepath.Join(e.RepoDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			e.t.Fatalf("WriteRepo MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			e.t.Fatalf("WriteRepo WriteFile: %v", err)
		}
	}
}

// InitGitRepo runs git init, git add -A, and git commit inside RepoDir.
func (e *Env) InitGitRepo() {
	e.t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = e.RepoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			e.t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("add", "-A")
	run("commit", "-m", "init")
}

// WriteHome creates files inside HomeDir. Keys are relative paths, values are content.
func (e *Env) WriteHome(files map[string]string) {
	e.t.Helper()
	for rel, content := range files {
		path := filepath.Join(e.HomeDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			e.t.Fatalf("WriteHome MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			e.t.Fatalf("WriteHome WriteFile: %v", err)
		}
	}
}

// MakeSymlink creates HomeDir/linkName → target.
func (e *Env) MakeSymlink(target, linkName string) {
	e.t.Helper()
	dest := filepath.Join(e.HomeDir, linkName)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		e.t.Fatalf("MakeSymlink MkdirAll: %v", err)
	}
	if err := os.Symlink(target, dest); err != nil {
		e.t.Fatalf("MakeSymlink: %v", err)
	}
}

// WriteConfig writes entries to ConfigPath as YAML.
func (e *Env) WriteConfig(entries []config.Entry) {
	e.t.Helper()
	cfg := &config.Config{Version: 1, BackupSuffix: e.BackupSuffix, Entries: entries}
	if err := config.Save(e.ConfigPath, cfg); err != nil {
		e.t.Fatalf("WriteConfig: %v", err)
	}
}

// AssertSymlink asserts HomeDir/name is a symlink pointing to wantTarget.
func (e *Env) AssertSymlink(t *testing.T, name, wantTarget string) {
	t.Helper()
	path := filepath.Join(e.HomeDir, name)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("AssertSymlink stat %s: %v", name, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("AssertSymlink: %s is not a symlink", name)
	}
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("AssertSymlink readlink %s: %v", name, err)
	}
	if got != wantTarget {
		t.Errorf("AssertSymlink %s: target = %q, want %q", name, got, wantTarget)
	}
}

// AssertRegular asserts HomeDir/name is a regular file with wantContent.
func (e *Env) AssertRegular(t *testing.T, name, wantContent string) {
	t.Helper()
	path := filepath.Join(e.HomeDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AssertRegular read %s: %v", name, err)
	}
	if string(data) != wantContent {
		t.Errorf("AssertRegular %s: content = %q, want %q", name, string(data), wantContent)
	}
}

// AssertBackup asserts HomeDir/name+BackupSuffix is a regular file with wantContent.
func (e *Env) AssertBackup(t *testing.T, name, wantContent string) {
	t.Helper()
	e.AssertRegular(t, name+e.BackupSuffix, wantContent)
}

// AssertNotExist asserts HomeDir/name does not exist.
func (e *Env) AssertNotExist(t *testing.T, name string) {
	t.Helper()
	path := filepath.Join(e.HomeDir, name)
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("AssertNotExist: %s exists but should not", name)
	}
}

// ReadYAML is a test-only helper to unmarshal a YAML file.
func ReadYAML(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadYAML: %v", err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("ReadYAML unmarshal: %v", err)
	}
}
