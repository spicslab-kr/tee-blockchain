package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	ra "tb/internal/tee/request/Vendor"
	awsra "tb/internal/tee/request/Vendor/aws"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ra-serve":
		err = raServe(os.Args[2:])
	case "ra-request":
		err = raRequest(os.Args[2:])
	case "ra-proxy":
		err = raProxy(os.Args[2:])
	case "lo-up":
		err = awsra.LoopbackUp()
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
	fmt.Fprintln(os.Stderr, `tb vendor — RA 요청

      ra-serve    --listen tcp:<host:port> | vsock:<port>  --extractor nsm | fixture=<doc>  [--user-data <hex>]
              TEE 안에서 돈다. 요청마다 난수를 받아 RA 문서를 돌려준다. fixture 는 aws-nitro 형식으로 표시된다
  lo-up       엔클레이브 안에서 loopback 을 올린다. 소프트웨어를 띄우기 전에 한 번
  ra-proxy    --listen <host:port>  --vsock <cid>:<port>
              Nitro 부모 인스턴스에서 돈다. 들어온 TCP 연결을 엔클레이브 vsock 으로 이어 준다
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

func raServe(args []string) error {
	fs := flag.NewFlagSet("ra-serve", flag.ContinueOnError)
	listen := fs.String("listen", "tcp:127.0.0.1:5000", "tcp:<host:port> 또는 vsock:<port>")
	extractor := fs.String("extractor", "nsm", "nsm 또는 fixture=<doc 파일>")
	userData := fs.String("user-data", "", "문서에 묶을 값 (hex)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var ex ra.Extractor
	switch {
	case *extractor == "nsm":
		ud, err := hex.DecodeString(*userData)
		if err != nil {
			return fmt.Errorf("--user-data: %w", err)
		}
		ex = awsra.Extractor{UserData: ud}
	case strings.HasPrefix(*extractor, "fixture="):
		fx, err := ra.LoadFixture(awsra.Format, strings.TrimPrefix(*extractor, "fixture="))
		if err != nil {
			return err
		}
		ex = fx
		fmt.Fprintln(os.Stderr, "warning: fixture extractor — the document is not bound to the nonce; test use only")
	default:
		return errors.New("--extractor: nsm 또는 fixture=<doc>")
	}
	var ln net.Listener
	var err error
	switch {
	case strings.HasPrefix(*listen, "tcp:"):
		ln, err = net.Listen("tcp", strings.TrimPrefix(*listen, "tcp:"))
	case strings.HasPrefix(*listen, "vsock:"):
		var port uint64
		port, err = strconv.ParseUint(strings.TrimPrefix(*listen, "vsock:"), 10, 32)
		if err == nil {
			ln, err = awsra.ListenVsock(uint32(port))
		}
	default:
		return errors.New("--listen: tcp:<host:port> 또는 vsock:<port>")
	}
	if err != nil {
		return err
	}
	fmt.Printf("ra-serve listening on %s\n", ln.Addr())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ra.Serve(ctx, ln, ex)
}

func raProxy(args []string) error {
	fs := flag.NewFlagSet("ra-proxy", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:5000", "받을 TCP 주소")
	target := fs.String("vsock", "16:5000", "엔클레이브 <cid>:<port>")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cidStr, portStr, ok := strings.Cut(*target, ":")
	cid, err1 := strconv.ParseUint(cidStr, 10, 32)
	port, err2 := strconv.ParseUint(portStr, 10, 32)
	if !ok || err1 != nil || err2 != nil {
		return errors.New("--vsock: <cid>:<port>")
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	fmt.Printf("ra-proxy %s -> vsock %d:%d\n", ln.Addr(), cid, port)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return awsra.Proxy(ctx, ln, uint32(cid), uint32(port))
}

func exitCode(err error) int {
	switch {
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
