package vendor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// These helpers run the installed git. Authentication comes from the user's git configuration.

var (
	ErrGitMissing      = errors.New("git is not installed or not on PATH")
	ErrRepoAccess      = errors.New("cannot access the repository: check the URL and read permission, or ask the repository owner for access")
	ErrCommitNotFound  = errors.New("the given commit is not in the repository")
	ErrPushRejected    = errors.New("push rejected: no write permission, or the remote is ahead (pull first)")
	ErrNothingToCommit = errors.New("nothing changed, nothing to commit")
)

type gitRunner struct{ dir string }

func (g gitRunner) run(ctx context.Context, args ...string) (string, error) {
	// autocrlf off keeps file bytes identical to the committed ones, so hashes match
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.autocrlf=false"}, args...)...)
	if g.dir != "" {
		cmd.Dir = g.dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0") // git fails when it needs credentials
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil {
		var ee *exec.Error
		if errors.As(err, &ee) {
			return "", ErrGitMissing
		}
		return out.String(), classify(errb.String(), err)
	}
	return strings.TrimSpace(out.String()), nil
}

// classify maps git's stderr text to one of the Err values above.
func classify(stderr string, err error) error {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "authentication failed"), strings.Contains(s, "permission denied"),
		strings.Contains(s, "could not read from remote"), strings.Contains(s, "repository not found"),
		strings.Contains(s, "does not appear to be a git repository"), strings.Contains(s, "403"), strings.Contains(s, "401"):
		return fmt.Errorf("%w: %s", ErrRepoAccess, strings.TrimSpace(stderr))
	case strings.Contains(s, "unknown revision"), strings.Contains(s, "did not match any"), strings.Contains(s, "bad object"),
		strings.Contains(s, "reference is not a tree"), strings.Contains(s, "not a valid object name"), strings.Contains(s, "unable to read tree"):
		return fmt.Errorf("%w: %s", ErrCommitNotFound, strings.TrimSpace(stderr))
	case strings.Contains(s, "rejected"), strings.Contains(s, "non-fast-forward"), strings.Contains(s, "failed to push"):
		return fmt.Errorf("%w: %s", ErrPushRejected, strings.TrimSpace(stderr))
	case strings.Contains(s, "nothing to commit"):
		return ErrNothingToCommit
	}
	return fmt.Errorf("git: %w: %s", err, strings.TrimSpace(stderr))
}

// cloneAt clones repo into dir, checks out ref when given, and returns the HEAD commit. An empty
// repository returns an empty commit.
func cloneAt(ctx context.Context, repo, ref, dir string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if _, err := (gitRunner{}).run(ctx, "clone", "--quiet", repo, dir); err != nil {
		return "", err
	}
	g := gitRunner{dir: dir}
	if ref != "" {
		if _, err := g.run(ctx, "checkout", "--quiet", "--detach", ref); err != nil {
			return "", err
		}
	}
	sha, err := g.run(ctx, "rev-parse", "HEAD")
	if err != nil && ref == "" {
		// An empty repository: the send becomes its first commit.
		if out, e2 := g.run(ctx, "rev-list", "-n", "1", "--all"); e2 == nil && strings.TrimSpace(out) == "" {
			return "", nil
		}
	}
	return sha, err
}

// commitAndPush stages paths, commits, pushes HEAD to origin and returns the commit.
func commitAndPush(ctx context.Context, dir, message string, paths ...string) (string, error) {
	g := gitRunner{dir: dir}
	if _, err := g.run(ctx, append([]string{"add", "--"}, paths...)...); err != nil {
		return "", err
	}
	if _, err := g.run(ctx, "commit", "--quiet", "-m", message); err != nil {
		return "", err
	}
	sha, err := g.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if _, err := g.run(ctx, "push", "--quiet", "origin", "HEAD"); err != nil {
		return "", err
	}
	return sha, nil
}
