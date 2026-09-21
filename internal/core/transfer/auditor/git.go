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

// GitTransport — Receive 는 소스 저장소를 clone 해 빌드 설정값을 받고, Send 는 측정값 기록과 이미지를 커밋 · push 한다.
type GitTransport struct {
	WorkDir string // 비어 있으면 Send 가 임시로 clone 한다
}

type SendResult struct {
	Repo   string
	Commit string
	Path   string
}

// Receive — repo 를 into 로 clone 하고 빌드 설정값을 읽는다. source.commit 이 checkout 이력에 있어야 한다.
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

// Send — 측정값 기록과 이미지 파일을 커밋 · push 한다.
func (g GitTransport) Send(ctx context.Context, rec MeasurementRecord, repo string) (SendResult, error) {
	if err := ctx.Err(); err != nil {
		return SendResult{}, err
	}
	dir := g.WorkDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "tb-auditor-send-*")
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
	sha, err := commitAndPush(ctx, dir, "tb: measurement-record for "+rec.Service+" "+rec.Version, paths...)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{Repo: repo, Commit: sha, Path: MeasurementRecordFile}, nil
}
