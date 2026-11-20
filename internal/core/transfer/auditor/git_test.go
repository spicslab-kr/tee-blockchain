package auditor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func TestGitSend_PushesRecordAndImage(t *testing.T) {
	remote := newRemote(t)
	build := t.TempDir()
	eif := filepath.Join(build, "svc.eif")
	if err := os.WriteFile(eif, []byte("eif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := recAWS()
	rec.Image = Image{Path: eif}
	work := clientDir(t, remote)
	res, err := GitTransport{WorkDir: work}.Send(context.Background(), rec, remote)
	if err != nil {
		t.Fatal(err)
	}
	check := filepath.Join(t.TempDir(), "check")
	run(t, "", "clone", "--quiet", remote, check)
	if head := strings.TrimSpace(run(t, check, "rev-parse", "HEAD")); head != res.Commit {
		t.Fatalf("remote HEAD %s != pushed %s", head, res.Commit)
	}
	for _, f := range []string{MeasurementRecordFile, MeasurementRecordFile + ".sha256", "svc.eif"} {
		if _, err := os.Stat(filepath.Join(check, f)); err != nil {
			t.Errorf("%s not in remote: %v", f, err)
		}
	}
}

// The same record and image sent again is a success with nothing committed: the remote already holds them. Before, git
// said "nothing to commit" on stdout, classify read stderr, and the send failed with an empty message
// (docs/results/git-transport-2026-11-20.md, 8).
func TestGitSend_SameContentAgainIsUpToDate(t *testing.T) {
	remote := newRemote(t)
	eif := filepath.Join(t.TempDir(), "svc.eif")
	if err := os.WriteFile(eif, []byte("eif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := recAWS()
	rec.Image = Image{Path: eif}
	first, err := GitTransport{WorkDir: clientDir(t, remote)}.Send(context.Background(), rec, remote)
	if err != nil || first.Unchanged {
		t.Fatalf("first send: %+v, %v", first, err)
	}
	again, err := GitTransport{WorkDir: clientDir(t, remote)}.Send(context.Background(), rec, remote)
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

// Into an empty repository the record becomes its first commit (cloneAt's branch for an empty repository).
// commitAndPush asks diff --cached on an unborn branch, which must count the staged files as a change.
func TestGitSend_EmptyRepoFirstCommit(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "empty.git")
	run(t, "", "init", "--quiet", "--bare", "--initial-branch=main", remote)
	work := filepath.Join(t.TempDir(), "work")
	run(t, "", "clone", "--quiet", remote, work)
	run(t, work, "config", "user.email", "t@t")
	run(t, work, "config", "user.name", "t")
	eif := filepath.Join(t.TempDir(), "svc.eif")
	if err := os.WriteFile(eif, []byte("eif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := recAWS()
	rec.Image = Image{Path: eif}
	res, err := GitTransport{WorkDir: work}.Send(context.Background(), rec, remote)
	if err != nil || res.Unchanged || res.Commit == "" {
		t.Fatalf("send into an empty repository: %+v, %v", res, err)
	}
	if head := strings.TrimSpace(run(t, "", "--git-dir", remote, "rev-parse", "main")); head != res.Commit {
		t.Errorf("remote main %s, want %s", head, res.Commit)
	}
}

func TestGitReceive_ClonesAndChecksSourceCommit(t *testing.T) {
	remote := newRemote(t)
	// Act as the Vendor: commit and push build-config.json (source.commit is the HEAD before that commit)
	work := clientDir(t, remote)
	prev := strings.TrimSpace(run(t, work, "rev-parse", "HEAD"))
	cfg := cfgAWS()
	cfg.Source = Source{Repo: remote, Commit: prev}
	b, _ := Encode(cfg)
	if err := os.WriteFile(filepath.Join(work, BuildConfigFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, "add", BuildConfigFile)
	run(t, work, "commit", "--quiet", "-m", "cfg")
	run(t, work, "push", "--quiet")
	head := strings.TrimSpace(run(t, work, "rev-parse", "HEAD"))

	into := filepath.Join(t.TempDir(), "auditor-clone")
	got, err := GitTransport{}.Receive(context.Background(), remote, head, into)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value.Source.Commit != prev {
		t.Errorf("source.commit = %s, want %s", got.Value.Source.Commit, prev)
	}
	if _, err := os.Stat(filepath.Join(into, "main.go")); err != nil {
		t.Error("vendor source must be present in the clone, since the auditor builds from it")
	}
}

func TestGitReceive_SourceCommitNotInHistoryRejected(t *testing.T) {
	remote := newRemote(t)
	work := clientDir(t, remote)
	cfg := cfgAWS()
	cfg.Source = Source{Repo: remote, Commit: "1111111111111111111111111111111111111111"}
	b, _ := Encode(cfg)
	_ = os.WriteFile(filepath.Join(work, BuildConfigFile), b, 0o644)
	run(t, work, "add", BuildConfigFile)
	run(t, work, "commit", "--quiet", "-m", "cfg")
	run(t, work, "push", "--quiet")
	if _, err := (GitTransport{}).Receive(context.Background(), remote, "", filepath.Join(t.TempDir(), "x")); err == nil || !strings.Contains(err.Error(), "not in the checked-out history") {
		t.Fatalf("want history check failure, got %v", err)
	}
}
