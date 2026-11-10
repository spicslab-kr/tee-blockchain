package auditor

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

// GitTransport is the git hand-off. Receive clones the source repository and reads the build config; Send
// commits and pushes the measurement record and its image.
type GitTransport struct {
	WorkDir string // an already-cloned working tree; when empty, Send clones repo into a temporary directory
}

// SendResult is the repository, the pushed commit and the file path within it.
type SendResult struct {
	Repo   string
	Commit string
	Path   string
}

// Receive clones repo at ref into the directory into and reads the build config. source.commit must be in
// the checked-out history.
func (g GitTransport) Receive(ctx context.Context, repo, ref, into string) (Received, error) {
	if err := ctx.Err(); err != nil {
		return Received{}, err
	}
	head, err := cloneAt(ctx, repo, ref, into)
	if err != nil {
		return Received{}, err
	}
	got, err := (FileTransport{}).Receive(ctx, filepath.Join(into, BuildConfigFile))
	if err != nil {
		return Received{}, fmt.Errorf("%s: %w", repo, err)
	}
	if _, err := (gitRunner{dir: into}).run(ctx, "merge-base", "--is-ancestor", got.Value.Source.Commit, head); err != nil {
		return Received{}, fmt.Errorf("build-config source.commit %s is not in the checked-out history (%s)", got.Value.Source.Commit, head)
	}
	return got, nil
}

// Send commits and pushes measurement-record.json, its .sha256 file and the image file.
func (g GitTransport) Send(ctx context.Context, rec MeasurementRecord, repo string) (SendResult, error) {
	if err := ctx.Err(); err != nil {
		return SendResult{}, err
	}
	dir := g.WorkDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "att-auditor-send-*")
		if err != nil {
			return SendResult{}, err
		}
		defer os.RemoveAll(tmp)
		dir = filepath.Join(tmp, "repo")
		if _, err := cloneAt(ctx, repo, "", dir); err != nil {
			return SendResult{}, err
		}
	}
	if _, err := (FileTransport{}).Send(ctx, rec, dir); err != nil {
		return SendResult{}, err
	}
	paths := []string{MeasurementRecordFile, MeasurementRecordFile + ".sha256"}
	if rec.Image.Path != "" {
		paths = append(paths, filepath.Base(rec.Image.Path))
	}
	sha, err := commitAndPush(ctx, dir, "att: measurement-record for "+rec.Service+" "+rec.Version, paths...)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{Repo: repo, Commit: sha, Path: MeasurementRecordFile}, nil
}
