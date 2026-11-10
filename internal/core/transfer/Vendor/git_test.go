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
