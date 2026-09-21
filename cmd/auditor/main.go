package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	tr "tb/internal/core/transfer/auditor"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "receive":
		err = receive(os.Args[2:])
	case "send":
		err = send(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tb auditor:", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `tb auditor — 파일 전달

  receive  --repo <url> [--ref <commit>] --into <dir>
           소스 저장소를 clone 해 빌드 설정값을 받는다. 이 디렉터리에서 빌드한다
  send     --repo <url> --record <measurement-record.json> [--image <file>] [--workdir <clone>]
           측정값 기록과 이미지를 저장소에 커밋 · push 한다

직접 하려면: setup/auditor/transfer.md`)
}

func receive(args []string) error {
	fs := flag.NewFlagSet("receive", flag.ContinueOnError)
	repo := fs.String("repo", "", "Vendor 소스 저장소 URL")
	ref := fs.String("ref", "", "커밋 · 브랜치 · 태그")
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
	c := got.Value
	fmt.Printf("received %s (sha256 %s)\n", got.Path, got.SHA256[:12])
	fmt.Printf("%s %s %s · source %s @ %s\n", c.Service, c.Version, c.Platform, c.Source.Repo, c.Source.Commit[:12])
	for k, v := range c.TEEInfo {
		fmt.Printf("  tee_info.%s = %s\n", k, v)
	}
	return nil
}

func send(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	repo := fs.String("repo", "", "올릴 저장소 URL")
	recPath := fs.String("record", "", "measurement-record.json 경로")
	image := fs.String("image", "", "이미지 파일 (aws: EIF). 기록의 image.path 를 덮어쓴다")
	work := fs.String("workdir", "", "이미 clone 된 작업 트리 (없으면 임시 clone)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *recPath == "" {
		return errors.New("--repo 와 --record 는 필수")
	}
	b, err := os.ReadFile(*recPath)
	if err != nil {
		return err
	}
	var rec tr.MeasurementRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return fmt.Errorf("measurement record: %w", err)
	}
	if *image != "" {
		rec.Image.Path = *image
	}
	res, err := tr.GitTransport{WorkDir: *work}.Send(context.Background(), rec, *repo)
	if err != nil {
		return err
	}
	fmt.Printf("sent %s → %s @ %s\n", res.Path, res.Repo, res.Commit)
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
