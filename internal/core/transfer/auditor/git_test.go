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

func TestGitReceive_ClonesAndChecksSourceCommit(t *testing.T) {
	remote := newRemote(t)
	// Vendor 역할을 흉내: build-config.json 을 커밋 · push (source.commit = 이전 HEAD)
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
		t.Error("vendor source must be present in the clone — auditor builds from here")
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
