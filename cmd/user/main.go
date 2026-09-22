package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	parse "tb/internal/tee/parse/user"
	ra "tb/internal/tee/request/user"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ra-parse":
		err = raParse(os.Args[2:])
	case "ra-request":
		err = raRequest(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tb user:", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `tb user — RA 요청

  ra-parse    --doc <file> [--nonce <hex|file>] [--root <pem>] [--format aws-nitro] [--out <file>] [--at <RFC3339>]
              문서의 서명 · 인증서 체인 · 난수를 확인하고 측정값(PCR)을 꺼낸다. 결과는 register --measurements 에 그대로 넣는다
  ra-request  --tee <host:port> [--out <file>] [--format <name>] [--timeout 30s]
          난수를 만들어 보내고 RA 문서를 받아 저장한다. 난수는 <file>.nonce 에.
              --format 을 주면 그 형식(aws-nitro · gcp-cs-jwt · azure-maa-jwt)의 문서만 받는다`)
}

func raRequest(args []string) error {
	fs := flag.NewFlagSet("ra-request", flag.ContinueOnError)
	tee := fs.String("tee", "", "TEE 주소 host:port")
	out := fs.String("out", "ra-doc.cbor", "문서를 저장할 파일")
	format := fs.String("format", "", "기대하는 문서 형식. 비우면 무엇이든 받는다")
	timeout := fs.Duration("timeout", 30*time.Second, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tee == "" {
		return errors.New("--tee 는 필수")
	}
	nonce, doc, err := ra.TCPRequester{Timeout: *timeout}.RequestFresh(context.Background(), *tee)
	if err != nil {
		return err
	}
	if err := doc.ExpectFormat(*format); err != nil {
		return err
	}
	if err := os.WriteFile(*out, doc.Raw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(*out+".nonce", []byte(hex.EncodeToString(nonce)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("received %s document, %d bytes → %s (nonce → %s.nonce)\n", doc.Format, len(doc.Raw), *out, *out)
	return nil
}

func exitCode(err error) int {
	switch {
	case errors.Is(err, parse.ErrWrongFormat):
		return 4
	case errors.Is(err, parse.ErrSignature), errors.Is(err, parse.ErrChain):
		return 5
	case errors.Is(err, parse.ErrNonce):
		return 6
	case errors.Is(err, parse.ErrBadDocument):
		return 7
	case errors.Is(err, ra.ErrWrongFormat):
		return 4
	case errors.Is(err, ra.ErrExtractFailed):
		return 5
	case errors.Is(err, ra.ErrBadResponse), errors.Is(err, ra.ErrTooLarge):
		return 6
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return 3
	}
	return 1
}
