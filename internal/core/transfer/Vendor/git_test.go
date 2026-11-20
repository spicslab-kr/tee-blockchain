package vendor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func TestGitSend_PushesBuildConfig(t *testing.T) {
	remote := newRemote(t)
	work := clientDir(t, remote)
	cfg := cfgAWS()
	cfg.Source = Source{Repo: "", Commit: ""} // filled in by Send
	res, err := GitTransport{WorkDir: work}.Send(context.Background(), cfg, remote)
	if err != nil {
		t.Fatal(err)
	}
	// Check with another clone that it reached the remote
	check := filepath.Join(t.TempDir(), "check")
	run(t, "", "clone", "--quiet", remote, check)
	if head := strings.TrimSpace(run(t, check, "rev-parse", "HEAD")); head != res.Commit {
		t.Fatalf("remote HEAD %s != pushed %s", head, res.Commit)
	}
	b, err := os.ReadFile(filepath.Join(check, BuildConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeBuildConfig(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source.Repo != remote || got.Source.Commit == "" || got.Source.Commit == res.Commit {
		t.Errorf("source must point at the repo and the commit before the config commit: %+v", got.Source)
	}
}

// The same build-config.json sent again is a success with nothing committed: the remote already holds it. Before, git
// said "nothing to commit" on stdout, classify read stderr, and the send failed with an empty message
// (docs/results/git-transport-2026-11-20.md, 8). source is written in, as Send would otherwise fill source.commit with
// the first send's commit and the content would differ.
func TestGitSend_SameContentAgainIsUpToDate(t *testing.T) {
	remote := newRemote(t)
	cfg := cfgAWS()
	first, err := GitTransport{WorkDir: clientDir(t, remote)}.Send(context.Background(), cfg, remote)
	if err != nil || first.Unchanged {
		t.Fatalf("first send: %+v, %v", first, err)
	}
	again, err := GitTransport{WorkDir: clientDir(t, remote)}.Send(context.Background(), cfg, remote)
	if err != nil {
		t.Fatalf("the same content again must succeed: %v", err)
	}
	if !again.Unchanged || again.Commit != first.Commit {
		t.Errorf("again: %+v, want Unchanged at %s", again, first.Commit)
	}
	check := filepath.Join(t.TempDir(), "check")
	run(t, "", "clone", "--quiet", remote, check)
	if head := strings.TrimSpace(run(t, check, "rev-parse", "HEAD")); head != first.Commit {
		t.Errorf("remote HEAD %s, want the first send's %s: nothing new is committed", head, first.Commit)
	}
}

// An empty repository holds no source for source.commit to name. Send says so, and what to do, instead of "the given
// commit is not in the repository" from its own rev-parse HEAD (docs/results/git-transport-2026-11-20.md, 7). The exit
// code stays 10: it is an ErrCommitNotFound.
func TestGitSend_EmptyRepoSaysPushSourceFirst(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "empty.git")
	run(t, "", "init", "--quiet", "--bare", "--initial-branch=main", remote)
	_, err := GitTransport{}.Send(context.Background(), cfgAWS(), remote)
	if !errorsIs(err, ErrEmptyRepo) || !errorsIs(err, ErrCommitNotFound) {
		t.Fatalf("want ErrEmptyRepo, an ErrCommitNotFound; got %v", err)
	}
	if !strings.Contains(err.Error(), "push the source") {
		t.Errorf("the message must say what to do: %v", err)
	}
	if out := strings.TrimSpace(run(t, "", "--git-dir", remote, "rev-list", "-n", "1", "--all")); out != "" {
		t.Errorf("nothing must reach the remote, it has %s", out)
	}
}

func TestGitSend_InvalidConfigNotPushed(t *testing.T) {
	remote := newRemote(t)
	work := clientDir(t, remote)
	cfg := cfgAWS()
	delete(cfg.TEEInfo, "ami_id")
	if _, err := (GitTransport{WorkDir: work}).Send(context.Background(), cfg, remote); err == nil {
		t.Fatal("invalid config must not be pushed")
	}
	check := filepath.Join(t.TempDir(), "check")
	run(t, "", "clone", "--quiet", remote, check)
	if _, err := os.Stat(filepath.Join(check, BuildConfigFile)); !os.IsNotExist(err) {
		t.Fatal("nothing must reach the remote")
	}
}
