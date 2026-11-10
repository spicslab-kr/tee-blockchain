package vendor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const (
	BuildConfigFile       = "build-config.json"
	MeasurementRecordFile = "measurement-record.json"
)

// GitTransport is the git hand-off. Send commits and pushes the build config; Receive clones the repository
// and reads the measurement record and its image.
type GitTransport struct {
	// WorkDir is an already-cloned source working tree. When empty, Send clones repo into a temporary directory.
	WorkDir string
}

// SendResult is the repository, the pushed commit and the file path within it.
type SendResult struct {
	Repo   string
	Commit string
	Path   string
}

// Send commits and pushes build-config.json at the repository root. An empty source.repo and source.commit
// are filled in from repo and the HEAD before this commit.
func (g GitTransport) Send(ctx context.Context, cfg BuildConfig, repo string) (SendResult, error) {
	if err := ctx.Err(); err != nil {
		return SendResult{}, err
	}
	dir := g.WorkDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "att-vendor-send-*")
		if err != nil {
			return SendResult{}, err
		}
		defer os.RemoveAll(tmp)
		dir = filepath.Join(tmp, "repo")
		if _, err := cloneAt(ctx, repo, "", dir); err != nil {
			return SendResult{}, err
		}
	}
	if cfg.Source.Repo == "" {
		cfg.Source.Repo = repo
	}
	head, err := gitRunner{dir: dir}.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return SendResult{}, err
	}
	if cfg.Source.Commit == "" {
		cfg.Source.Commit = head
	}
	if err := cfg.Validate(); err != nil {
		return SendResult{}, err
	}
	b, err := Encode(cfg)
	if err != nil {
		return SendResult{}, err
	}
	path := filepath.Join(dir, BuildConfigFile)
	if err := writeAtomic(path, b); err != nil {
		return SendResult{}, err
	}
	sha, err := commitAndPush(ctx, dir, "att: build-config for "+cfg.Service+" "+cfg.Version, BuildConfigFile)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{Repo: repo, Commit: sha, Path: BuildConfigFile}, nil
}

// Receive clones repo at ref into the directory into, then reads the measurement record and checks the image against its sha256.
func (g GitTransport) Receive(ctx context.Context, repo, ref, into string) (Received, error) {
	if err := ctx.Err(); err != nil {
		return Received{}, err
	}
	if _, err := cloneAt(ctx, repo, ref, into); err != nil {
		return Received{}, err
	}
	rec, err := (FileTransport{}).Receive(ctx, filepath.Join(into, MeasurementRecordFile))
	if err != nil {
		return Received{}, fmt.Errorf("%s: %w", repo, err)
	}
	return rec, nil
}
