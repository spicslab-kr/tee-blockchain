package vendor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A local bare repository serves as the remote. Real git runs.
func newRemote(t *testing.T) (remote string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	remote = filepath.Join(t.TempDir(), "remote.git")
	run(t, "", "init", "--quiet", "--bare", "--initial-branch=main", remote)
	seed := filepath.Join(t.TempDir(), "seed")
	run(t, "", "clone", "--quiet", remote, seed)
	run(t, seed, "config", "user.email", "t@t")
	run(t, seed, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(seed, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, seed, "add", "main.go")
	run(t, seed, "commit", "--quiet", "-m", "seed")
	run(t, seed, "push", "--quiet", "-u", "origin", "main")
	return remote
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func clientDir(t *testing.T, remote string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "work")
	run(t, "", "clone", "--quiet", remote, dir)
	run(t, dir, "config", "user.email", "t@t")
	run(t, dir, "config", "user.name", "t")
	return dir
}

func TestGit_RepoAccessError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, err := cloneAt(context.Background(), filepath.Join(t.TempDir(), "does-not-exist.git"), "", filepath.Join(t.TempDir(), "x"))
	if err == nil {
		t.Fatal("clone of a missing repo must fail")
	}
	if !errorsIs(err, ErrRepoAccess) {
		t.Fatalf("want ErrRepoAccess, got %v", err)
	}
}

func TestGit_CommitNotFound(t *testing.T) {
	remote := newRemote(t)
	_, err := cloneAt(context.Background(), remote, "0000000000000000000000000000000000000000", filepath.Join(t.TempDir(), "x"))
	if !errorsIs(err, ErrCommitNotFound) {
		t.Fatalf("want ErrCommitNotFound, got %v", err)
	}
}
