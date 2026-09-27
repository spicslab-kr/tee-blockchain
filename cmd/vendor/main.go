package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
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

	reg "tb/internal/chain/register/Vendor"
	"tb/internal/cli"
	tr "tb/internal/core/transfer/Vendor"
	build "tb/internal/setup/build/Vendor/aws"
	launch "tb/internal/setup/launch/Vendor/aws"
	parse "tb/internal/tee/parse/Vendor"
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
	case "registry-deploy":
		err = registryDeploy(os.Args[2:])
	case "register":
		_, err = register(os.Args[2:])
	case "lookup":
		_, err = lookup(os.Args[2:])
	case "compare":
		err = compareCmd(os.Args[2:])
	case "self-register":
		err = selfRegisterCmd(os.Args[2:])
	case "send":
		err = send(os.Args[2:])
	case "receive":
		err = receive(os.Args[2:])
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
	case "pack":
		err = packCmd(os.Args[2:])
	case "launch":
		err = launchCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		cli.Errorln("tb vendor: ", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `tb vendor

  registry-deploy  --rpc <url> --key-file <file> --approver <0x…>
                   deploy the Registry contract. one signer per contract, fixed at deploy time
  register         --rpc <url> --contract <0x…> --key-file <file> --measurements <measurements.json | NAME=hex,…>
                   --service <name> --version <v> [--platform aws-nitro] [--expires 2160h]
                   sign the measurements with my key and register them on the contract. the key in the output is the lookup coordinate
  lookup           --rpc <url> --contract <0x…> (--key <0x…> | --measurements <measurements.json | NAME=hex,…>)
                   read one key directly, or find the key(s) for the measurements first and read them all
  compare          --a <measurements.json | NAME=hex,…> --b <same shape>
                   match two sets of measurements as they are (parsed vs built, etc.). prints PASS/FAIL with the reasons
  compare          --a <measurements.json | NAME=hex,…> --rpc <url> --contract <0x…> --key <0x…> [--trusted <0x…,0x…>]
                   read the registration from the chain and check the value, expiry and trusted signer together. exit 1 on failure
  self-register    [--tee <my enclave host:port>] [--measurements <file>] --rpc <url> --contract <0x…> --key-file <file>
                   --service <name> --version <v>
                   flow 2, no Auditor: attest my own enclave (T1·T2), register the value under my key (C1), read it back and
                   self-check (C2·M2). with --measurements too, registers only if the attested value matches it. --nonce pins
                   the nonce. with --service/--version empty, takes them from an existing registration on this contract
  send             --repo <url> --config <build-config.json> [--workdir <clone>]
                   commit and push the build config to the source repository
  receive          --repo <url> [--ref <commit>] --into <dir>
                   clone the measurement record and image, and verify them
  build       --app-dir <dir with the software's Dockerfile>  [--name <n>] [--port 5000] [--out out] [--responder <tb-ra-responder>] [--verify]
              on the Nitro parent instance. builds the software image, layers the RA responder on top, and writes the EIF and measurements (PCR0·1·2).
              for source that an Auditor will rebuild the same way (Case B)
  pack        --image <docker image you built>  [--name <n>] [--port 5000] [--out out] [--responder <tb-ra-responder>]
              same as build minus the docker build: takes your image, layers the responder, writes the EIF and measurements.
              for an image you hand to the Auditor as a file (Case A)
  launch      --eif <file>  [--measurements <file>] [--name <n>] [--port 5000] [--memory <MiB>] [--cpus 2] [--cid 16] [--listen 0.0.0.0] [--out out]
              on the Nitro parent instance. measures the EIF (→ out/measurements.json), runs it as an enclave, bridges TCP→vsock,
              requests RA from itself to confirm, then opens the security-group port (when permitted).
              --measurements is for a received EIF: the file it came with; the EIF is launched only if it measures to that
  ra-serve    --listen tcp:<host:port> | vsock:<port>  --extractor nsm | fixture=<doc>  [--user-data <hex>]
              runs inside the TEE. takes a nonce per request and returns an RA document. a fixture is labelled aws-nitro
  lo-up       brings up loopback inside the enclave. once, before starting the software
  ra-proxy    --listen <host:port>  --vsock <cid>:<port>
              runs on the Nitro parent instance. forwards incoming TCP connections to the enclave's vsock
  ra-parse    --doc <file> [--nonce <hex|file>] [--root <pem>] [--format aws-nitro] [--out <file>] [--at <RFC3339>]
              verifies the document's signature, certificate chain and nonce, and extracts the measurements (PCR). the output goes straight into register --measurements
  ra-request  --tee <host:port> [--out <file>] [--format <name>] [--timeout 30s]
              generates a nonce, sends it, and saves the RA document. the nonce goes to <file>.nonce.
              with --format, accepts only a document of that format (aws-nitro · gcp-cs-jwt · azure-maa-jwt)

to do it by hand: setup/vendor/transfer.md`)
}

func send(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	repo := fs.String("repo", "", "source repository URL")
	cfgPath := fs.String("config", "", "path to build-config.json")
	work := fs.String("workdir", "", "an existing clone to work in (default: a temporary clone)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *cfgPath == "" {
		return errors.New("--repo and --config are required")
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
	repo := fs.String("repo", "", "Auditor repository URL")
	ref := fs.String("ref", "", "commit, branch or tag (default: the default branch)")
	into := fs.String("into", "", "directory to clone into")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *into == "" {
		return errors.New("--repo and --into are required")
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

func raRequest(args []string) error {
	fs := flag.NewFlagSet("ra-request", flag.ContinueOnError)
	tee := fs.String("tee", "", "TEE address host:port")
	out := fs.String("out", "ra-doc.cbor", "file to save the document to")
	format := fs.String("format", "", "expected document format. empty accepts any")
	timeout := fs.Duration("timeout", 30*time.Second, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tee == "" {
		return errors.New("--tee is required")
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
	listen := fs.String("listen", "tcp:127.0.0.1:5000", "tcp:<host:port> or vsock:<port>")
	extractor := fs.String("extractor", "nsm", "nsm or fixture=<doc file>")
	userData := fs.String("user-data", "", "value to bind into the document (hex)")
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
		return errors.New("--extractor: nsm or fixture=<doc>")
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
		return errors.New("--listen: tcp:<host:port> or vsock:<port>")
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
	fs.StringVar(&b.AppDir, "app-dir", "", "directory with the software's Dockerfile")
	fs.StringVar(&b.Name, "name", "", "image and EIF name (default: the directory name)")
	fs.IntVar(&b.Port, "port", 5000, "port the responder listens on inside the enclave")
	fs.StringVar(&b.Out, "out", "out", "output directory")
	fs.StringVar(&b.Binary, "responder", "", "path to the T1 responder (tb-ra-responder). default: tb-ra-responder next to this executable")
	fs.BoolVar(&b.Verify, "verify", false, "build the Dockerfile a second time without cache and fail unless PCR0·1·2 come out the same, listing the files that differed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if b.AppDir == "" {
		return errors.New("--app-dir is required")
	}
	built, err := b.Run(context.Background(), os.Stdout)
	if err != nil {
		return err
	}
	fmt.Printf("\ndone. for the measurement record: %s · %s\nto launch:  tb vendor launch --eif %s --port %d\n", built.EIF, built.Measurements, built.EIF, b.Port)
	return nil
}

func packCmd(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	b := build.Build{}
	fs.StringVar(&b.Image, "image", "", "a docker image you built (name:tag)")
	fs.StringVar(&b.Name, "name", "", "EIF name (default: the image name without its tag)")
	fs.IntVar(&b.Port, "port", 5000, "port the responder listens on inside the enclave")
	fs.StringVar(&b.Out, "out", "out", "output directory")
	fs.StringVar(&b.Binary, "responder", "", "path to the T1 responder (tb-ra-responder). default: tb-ra-responder next to this executable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if b.Image == "" {
		return errors.New("--image is required")
	}
	built, err := b.Run(context.Background(), os.Stdout)
	if err != nil {
		return err
	}
	fmt.Printf("\ndone. hand these to the Auditor: %s · %s\nto launch:  tb vendor launch --eif %s --port %d\n", built.EIF, built.Measurements, built.EIF, b.Port)
	return nil
}

func launchCmd(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	l := launch.Launch{}
	fs.StringVar(&l.EIF, "eif", "", "EIF file to run")
	fs.StringVar(&l.Name, "name", "", "enclave name (default: the EIF file name)")
	fs.IntVar(&l.Port, "port", 5000, "RA port. must match the value given to build")
	fs.IntVar(&l.Memory, "memory", 0, "enclave memory in MiB (0: sized from the EIF)")
	fs.IntVar(&l.CPUs, "cpus", 2, "enclave CPU count")
	fs.IntVar(&l.CID, "cid", 16, "enclave vsock CID")
	fs.StringVar(&l.Listen, "listen", "0.0.0.0", "address the bridge listens on")
	fs.StringVar(&l.Out, "out", "out", "directory for logs and the self-check document")
	fs.StringVar(&l.Measurements, "measurements", "", "the measurements.json that came with a received EIF: the EIF is measured itself and must match, or it is not launched")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if l.EIF == "" {
		return errors.New("--eif is required")
	}
	ctx := context.Background()
	started, err := l.Run(ctx, os.Stdout)
	if err != nil {
		return err
	}

	fmt.Println("== self-check: requesting RA from this instance", started.Address)
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

	fmt.Println("== security group: inbound TCP", l.Port)
	cmd, group, perr := launch.OpenPort(ctx, l.Port, os.Stdout)
	if perr == nil {
		fmt.Printf("\ndone. anyone can now:  tb user ra-request --tee <this instance's public IP>:%d --format aws-nitro\n", l.Port)
		return nil
	}
	// 여기서는 못 열었다 — 무엇을, 어디서, 어떻게 열어야 하는지까지 적어 준다. 권한이 없는 경우와
	// 그 밖의 실패(메타데이터 · aws CLI)를 두 번째 줄에서만 가른다.
	why := "This instance has no permission to change it. From an AWS account that does, run:"
	if !errors.Is(perr, launch.ErrNoPermission) {
		why = fmt.Sprintf("Could not open it from here (%v). From an AWS account that can, run:", perr)
	}
	if group == "" {
		group = "<this instance's security group>"
	}
	fmt.Printf(`
Inbound TCP %d must be open on this instance's security group so the Auditor and Users can reach the enclave.
%s
  %s
or open it in the console: EC2 → Security Groups → %s → Inbound rules → Custom TCP %d, source 0.0.0.0/0.
Then anyone can:  tb user ra-request --tee <this instance's public IP>:%d --format aws-nitro
`, l.Port, why, cmd, group, l.Port, l.Port)
	return nil
}

func raProxy(args []string) error {
	fs := flag.NewFlagSet("ra-proxy", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:5000", "TCP address to listen on")
	target := fs.String("vsock", "16:5000", "enclave <cid>:<port>")
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
	case errors.Is(err, tr.ErrGitMissing):
		return 8
	case errors.Is(err, tr.ErrRepoAccess):
		return 9
	case errors.Is(err, tr.ErrCommitNotFound):
		return 10
	case errors.Is(err, tr.ErrPushRejected):
		return 11
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return 3
	}
	return 1
}
