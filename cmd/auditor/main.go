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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	reg "tb/internal/chain/register/auditor"
	build "tb/internal/setup/build/auditor/aws"
	launch "tb/internal/setup/launch/auditor/aws"
	parse "tb/internal/tee/parse/auditor"
	ra "tb/internal/tee/request/auditor"
	awsra "tb/internal/tee/request/auditor/aws"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "registry-deploy":
		err = registryDeploy(os.Args[2:])
	case "register":
		err = register(os.Args[2:])
	case "lookup":
		err = lookup(os.Args[2:])
	case "compare":
		err = compareCmd(os.Args[2:])
	case "ra-serve":
		err = raServe(os.Args[2:])
	case "ra-parse":
		err = raParse(os.Args[2:])
	case "ra-request":
		err = raRequest(os.Args[2:])
	case "ra-proxy":
		err = raProxy(os.Args[2:])
	case "lo-up":
		err = awsra.LoopbackUp()
	case "build":
		err = buildCmd(os.Args[2:])
	case "launch":
		err = launchCmd(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, `tb auditor

  registry-deploy  --rpc <url> --key-file <file> --auditor <0x…> --vendor <0x…>
                   Registry 컨트랙트를 배포한다. 두 주소는 그 뒤 바꿀 수 없다
  register         --rpc <url> --contract <0x…> --key-file <file> --measurements <measurements.json | NAME=hex,…>
                   --service <name> --version <v> [--platform aws-nitro] [--expires 2160h]
                   측정값을 내 키로 서명해 컨트랙트에 등록한다. 결과의 key 가 조회 좌표다
  lookup           --rpc <url> --contract <0x…> (--key <0x…> | --measurements <measurements.json | NAME=hex,…>)
                   key 로 바로 읽거나, 측정값으로 key 를 먼저 찾은 뒤 전부 읽는다
  compare          --a <measurements.json | NAME=hex,…> --b <같은 모양>
                   측정값 두 묶음을 그대로 맞댄다 (파싱값 vs 빌드값 등). PASS/FAIL 과 근거를 낸다
  compare          --a <measurements.json | NAME=hex,…> --rpc <url> --contract <0x…> --key <0x…> [--trusted <0x…,0x…>]
                   체인에서 등록 정보를 읽어 값 · 만료 · 신뢰 서명자까지 같이 본다. 실패하면 exit 1
  build       --app-dir <소프트웨어 Dockerfile 디렉터리>  [--name <n>] [--port 5000] [--out out]
              Nitro 부모 인스턴스에서. 소프트웨어 이미지를 빌드하고 그 위에 RA 응답기를 얹어 EIF 와 측정값(PCR0·1·2)을 낸다
  launch      --eif <file>  [--name <n>] [--port 5000] [--memory <MiB>] [--cpus 2] [--cid 16] [--listen 0.0.0.0] [--out out]
              Nitro 부모 인스턴스에서. EIF 를 엔클레이브로 띄우고 TCP→vsock 다리를 놓고, 자기 자신에게 RA 요청해 확인한 뒤
              보안 그룹 포트를 연다(권한이 있을 때). 받은 EIF 를 띄우기만 할 때도 이것
  ra-serve    --listen tcp:<host:port> | vsock:<port>  --extractor nsm | fixture=<doc>  [--user-data <hex>]
              TEE 안에서 돈다. 요청마다 난수를 받아 RA 문서를 돌려준다. fixture 는 aws-nitro 형식으로 표시된다
  lo-up       엔클레이브 안에서 loopback 을 올린다. 소프트웨어를 띄우기 전에 한 번
  ra-proxy    --listen <host:port>  --vsock <cid>:<port>
              Nitro 부모 인스턴스에서 돈다. 들어온 TCP 연결을 엔클레이브 vsock 으로 이어 준다
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

func buildCmd(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	b := build.Build{}
	fs.StringVar(&b.AppDir, "app-dir", "", "소프트웨어 Dockerfile 이 있는 디렉터리")
	fs.StringVar(&b.Name, "name", "", "이미지 · EIF 이름 (기본: 디렉터리 이름)")
	fs.IntVar(&b.Port, "port", 5000, "응답기가 엔클레이브 안에서 들을 포트")
	fs.StringVar(&b.Out, "out", "out", "결과 디렉터리")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if b.AppDir == "" {
		return errors.New("--app-dir 는 필수")
	}
	built, err := b.Run(context.Background(), os.Stdout)
	if err != nil {
		return err
	}
	fmt.Printf("\n끝. 측정값 기록에 적을 것: %s · %s\n띄우려면:  tb auditor launch --eif %s --port %d\n", built.EIF, built.Measurements, built.EIF, b.Port)
	return nil
}

func launchCmd(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	l := launch.Launch{}
	fs.StringVar(&l.EIF, "eif", "", "띄울 EIF 파일")
	fs.StringVar(&l.Name, "name", "", "엔클레이브 이름 (기본: EIF 파일 이름)")
	fs.IntVar(&l.Port, "port", 5000, "RA 포트. build 때 준 값과 같아야 한다")
	fs.IntVar(&l.Memory, "memory", 0, "엔클레이브 메모리 MiB (0: EIF 크기로 정함)")
	fs.IntVar(&l.CPUs, "cpus", 2, "엔클레이브 CPU 수")
	fs.IntVar(&l.CID, "cid", 16, "엔클레이브 vsock CID")
	fs.StringVar(&l.Listen, "listen", "0.0.0.0", "다리가 받을 주소")
	fs.StringVar(&l.Out, "out", "out", "로그 · 확인 문서 디렉터리")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if l.EIF == "" {
		return errors.New("--eif 는 필수")
	}
	ctx := context.Background()
	started, err := l.Run(ctx, os.Stdout)
	if err != nil {
		return err
	}

	fmt.Println("== 확인: 이 인스턴스에서 RA 요청", started.Address)
	nonce, doc, err := ra.TCPRequester{Timeout: 30 * time.Second}.RequestFresh(ctx, started.Address)
	if err != nil {
		return err
	}
	if err := doc.ExpectFormat(awsra.Format); err != nil {
		return err
	}
	selfDoc := filepath.Join(l.Out, "selfcheck.cbor")
	if err := os.WriteFile(selfDoc, doc.Raw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(selfDoc+".nonce", []byte(hex.EncodeToString(nonce)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("received %s document, %d bytes → %s\n", doc.Format, len(doc.Raw), selfDoc)

	fmt.Println("== 보안 그룹에 TCP", l.Port, "인바운드")
	cmd, perr := launch.OpenPort(ctx, l.Port, os.Stdout)
	switch {
	case perr == nil:
		fmt.Printf("\n끝. 누구나:  tb user ra-request --tee <이 인스턴스 공인 IP>:%d --format aws-nitro\n", l.Port)
	case errors.Is(perr, launch.ErrNoPermission):
		fmt.Printf("\n이 인스턴스에는 보안 그룹을 바꿀 권한이 없다. AWS 계정에서 한 줄:\n  %s\n그 뒤 누구나:  tb user ra-request --tee <이 인스턴스 공인 IP>:%d --format aws-nitro\n", cmd, l.Port)
	default:
		fmt.Printf("\n보안 그룹을 열지 못했다 (%v). AWS 계정에서 한 줄:\n  %s\n", perr, cmd)
	}
	return nil
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
	case errors.Is(err, reg.ErrNotAnApprover):
		return 4
	case errors.Is(err, reg.ErrAlreadyRegistered):
		return 5
	case errors.Is(err, reg.ErrExpiryPassed):
		return 6
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
