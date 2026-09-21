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

// GitTransport — Send 는 빌드 설정값을 커밋 · push 하고, Receive 는 저장소를 clone 해 측정값 기록과 이미지를 받는다.
type GitTransport struct {
	// WorkDir 는 이미 clone 된 소스 작업 트리. 비어 있으면 Send 가 Repo 를 임시로 clone 한다.
	WorkDir string
}

// SendResult — 올라간 저장소와 커밋.
type SendResult struct {
	Repo   string
	Commit string
	Path   string
}

// Send — build-config.json 을 저장소 루트에 커밋 · push 한다. source.commit 은 직전 HEAD 로 채운다.
func (g GitTransport) Send(ctx context.Context, cfg BuildConfig, repo string) (SendResult, error) {
	if err := ctx.Err(); err != nil {
		return SendResult{}, err
	}
	dir := g.WorkDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "tb-vendor-send-*")
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
	sha, err := commitAndPush(ctx, dir, "tb: build-config for "+cfg.Service+" "+cfg.Version, BuildConfigFile)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{Repo: repo, Commit: sha, Path: BuildConfigFile}, nil
}

// Receive — repo 를 into 로 clone 하고 측정값 기록과 이미지를 검증한다.
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
