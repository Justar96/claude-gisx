package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitInfo reads branch and dirtiness out of a single porcelain-v2 call; these
// pin the cases where that parsing could drift from the old three-command form.
func TestGitInfo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=", "GIT_CONFIG_SYSTEM=")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	write("a.txt", "a")
	run("add", ".")
	run("commit", "-qm", "init")

	if g := gitInfo(dir); g.branch != "main" || g.dirty != "" {
		t.Errorf("clean repo: got %+v, want branch=main dirty=''", g)
	}

	write("untracked.txt", "x")
	if g := gitInfo(dir); g.dirty != "*" {
		t.Errorf("an untracked file should read as dirty, got %+v", g)
	}
	if err := os.Remove(filepath.Join(dir, "untracked.txt")); err != nil {
		t.Fatal(err)
	}

	write("a.txt", "changed")
	if g := gitInfo(dir); g.dirty != "*" {
		t.Errorf("a modified file should read as dirty, got %+v", g)
	}
	run("add", ".")
	if g := gitInfo(dir); g.dirty != "*" {
		t.Errorf("a staged change should read as dirty, got %+v", g)
	}
	run("commit", "-qm", "two")

	run("checkout", "-q", "-b", "feature/x")
	if g := gitInfo(dir); g.branch != "feature/x" {
		t.Errorf("slashed branch name mangled: %+v", g)
	}

	// Detached HEAD reported no branch under symbolic-ref; keep that.
	run("checkout", "-q", "--detach")
	if g := gitInfo(dir); g.branch != "" {
		t.Errorf("detached HEAD should have no branch, got %+v", g)
	}

	if g := gitInfo(t.TempDir()); g.branch != "" || g.dirty != "" {
		t.Errorf("outside a work tree everything should be empty, got %+v", g)
	}
}
