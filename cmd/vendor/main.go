package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	tr "tb/internal/core/transfer/Vendor"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "send":
		err = send(os.Args[2:])
	case "receive":
		err = receive(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tb vendor:", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `tb vendor — 파일 전달

  send     --repo <url> --config <build-config.json> [--workdir <clone>]
           빌드 설정값을 소스 저장소에 커밋 · push 한다
  receive  --repo <url> [--ref <commit>] --into <dir>
           측정값 기록과 이미지를 clone 해 검증한다

직접 하려면: setup/vendor/transfer.md`)
}

func send(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	repo := fs.String("repo", "", "소스 저장소 URL")
	cfgPath := fs.String("config", "", "build-config.json 경로")
	work := fs.String("workdir", "", "이미 clone 된 작업 트리 (없으면 임시 clone)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *cfgPath == "" {
		return errors.New("--repo 와 --config 는 필수")
	}
	b, err := os.ReadFile(*cfgPath)
	if err != nil {
		return err
	}
	var cfg tr.BuildConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("build config: %w", err)
	}
	res, err := tr.GitTransport{WorkDir: *work}.Send(context.Background(), cfg, *repo)
	if err != nil {
		return err
	}
	fmt.Printf("sent %s → %s @ %s\n", res.Path, res.Repo, res.Commit)
	return nil
}

func receive(args []string) error {
	fs := flag.NewFlagSet("receive", flag.ContinueOnError)
	repo := fs.String("repo", "", "Auditor 저장소 URL")
	ref := fs.String("ref", "", "커밋 · 브랜치 · 태그 (없으면 기본 브랜치)")
	into := fs.String("into", "", "clone 할 디렉터리")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *into == "" {
		return errors.New("--repo 와 --into 는 필수")
	}
	got, err := tr.GitTransport{}.Receive(context.Background(), *repo, *ref, *into)
	if err != nil {
		return err
	}
	fmt.Printf("received %s (sha256 %s)\n", got.Path, got.SHA256[:12])
	if got.ImagePath != "" {
		fmt.Printf("image    %s (verified)\n", got.ImagePath)
	}
	fmt.Printf("%s %s %s · %d measurements · %s %s\n", got.Value.Service, got.Value.Version, got.Value.Platform,
		len(got.Value.Values), got.Value.BuildTool.Name, got.Value.BuildTool.Version)
	return nil
}

func exitCode(err error) int {
	switch {
	case errors.Is(err, tr.ErrGitMissing):
		return 3
	case errors.Is(err, tr.ErrRepoAccess):
		return 4
	case errors.Is(err, tr.ErrCommitNotFound):
		return 5
	case errors.Is(err, tr.ErrPushRejected):
		return 6
	}
	return 1
}
