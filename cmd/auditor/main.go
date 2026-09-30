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

	reg "tb/internal/chain/register/auditor"
	"tb/internal/cli"
	tr "tb/internal/core/transfer/auditor"
	toolchain "tb/internal/setup/build"
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
		_, err = register(os.Args[2:])
	case "lookup":
		_, err = lookup(os.Args[2:])
	case "compare":
		err = compareCmd(os.Args[2:])
	case "audit":
		err = auditCmd(os.Args[2:])
	case "receive":
		err = receive(os.Args[2:])
	case "send":
		err = send(os.Args[2:])
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
	case "version":
		fmt.Println("tb auditor ·", toolchain.ToolchainLine())
		err = toolchain.CheckToolchain()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		cli.Errorln("tb auditor: ", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `tb auditor

  version          print the pinned Go toolchain (internal/setup/build/GO_TOOLCHAIN) and the one this binary was built with;
                   exit 1 if they differ — build/pack refuse to run in that case, since the responder's bytes and the measurements depend on it
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
  audit            --tee <my enclave host:port> --measurements <my out/measurements.json> --rpc <url> --contract <0x…> --key-file <file>
                   --service <name> --version <v> [--vendor-tee <the Vendor's enclave host:port>]
                   flow 1 in one go: attest my own enclave (T1·T2), check the attested value against --measurements (the value I
                   built or measured myself), register that value only if they match (C1), then read it back and self-check (C2·M2).
                   --nonce pins the nonce. with --service/--version empty, takes them from an existing registration of the same
                   measurements on this contract. without --tee registers --measurements as is (no audit).
                   --vendor-tee also asks the Vendor's running enclave and says whether it runs the same value (flow 2 step 15); what is
                   registered is still my value
  receive          --repo <url> [--ref <commit>] --into <dir> [--ignore-env]
                   clone the source repository and read the build config. build from that directory
  send             --repo <url> --record <measurement-record.json> [--image <file>] [--workdir <clone>]
                   commit and push the measurement record and image to the repository. the record must say which CPU
                   architecture it is for ("arch": amd64 | arm64) — measurements differ per architecture
  build       --app-dir <dir with the software's Dockerfile>  [--name <n>] [--port 5000] [--out out] [--responder <tb-ra-responder>] [--verify]
              on the Nitro parent instance. builds the software image, layers the RA responder on top, and writes the EIF and measurements (PCR0·1·2).
              for source that an Auditor will rebuild the same way (Case B)
  pack        --image <docker image you built>  [--name <n>] [--port 5000] [--out out] [--responder <tb-ra-responder>]
              same as build minus the docker build: takes your image, layers the responder, writes the EIF and measurements.
              for an image handed over as a file (Case A)
  launch      --eif <file>  [--measurements <file>] [--name <n>] [--port 5000] [--memory <MiB>] [--cpus 2] [--cid 16] [--listen 0.0.0.0] [--out out]
              on the Nitro parent instance. runs the EIF as an enclave, bridges TCP→vsock,
              requests RA from itself to confirm, then opens the security-group port (when permitted).
              --measurements is for a received EIF: the file it came with. the EIF is then measured itself (→ out/measurements.json) and launched only if it measures to that
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

to do it by hand: setup/auditor/transfer.md`)
}

func receive(args []string) error {
	fs := flag.NewFlagSet("receive", flag.ContinueOnError)
	repo := fs.String("repo", "", "Vendor source repository URL")
	ref := fs.String("ref", "", "commit, branch or tag")
	into := fs.String("into", "", "directory to clone into")
	ignoreEnv := fs.Bool("ignore-env", false, "only warn when this instance's OS/architecture/Go differ from the Vendor's build_env")
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
	c := got.Value
	fmt.Printf("received %s (sha256 %s)\n", got.Path, got.SHA256[:12])
	fmt.Printf("%s %s %s · source %s @ %s\n", c.Service, c.Version, c.Platform, c.Source.Repo, c.Source.Commit[:12])
	for k, v := range c.TEEInfo {
		fmt.Printf("  tee_info.%s = %s\n", k, v)
	}
	// the Vendor said which environment it built in. this instance must be the same one, or
	// a rebuild here can't reproduce its measurements — and would look like a lying Vendor.
	have := tr.CurrentBuildEnv()
	fmt.Printf("build env: %s (Vendor)\n", c.BuildEnv)
	fmt.Printf("build env: %s (this instance)\n", have)
	warnings, err := tr.CheckBuildEnv(c.BuildEnv, have, *ignoreEnv)
	for _, w := range warnings {
		cli.Errorf("warning: %s", w)
	}
	if err != nil {
		return fmt.Errorf("%w (rerun with --ignore-env to proceed anyway)", err)
	}
	if c.BuildEnv.OS == have.OS && c.BuildEnv.Arch == have.Arch && c.BuildEnv.Go == have.Go {
		cli.Okln("build env matches the Vendor's")
	}
	return nil
}

func send(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	repo := fs.String("repo", "", "repository URL to push to")
	recPath := fs.String("record", "", "path to measurement-record.json")
	image := fs.String("image", "", "image file (aws: the EIF). overrides image.path in the record")
	work := fs.String("workdir", "", "an existing clone to work in (default: a temporary clone)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *recPath == "" {
		return errors.New("--repo and --record are required")
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
	// clone 하고 이미지를 해시하기 전에 기록부터 거른다 (Send 도 다시 검사한다). 이미지 해시는 Send 가 채우니 그 자리만 메워서 본다.
	// arch 가 빠진 기록은 어느 아키텍처 것인지 알 수 없으니 보내지 않는다.
	chk := rec
	if chk.Image.Path != "" && chk.Image.SHA256 == "" {
		chk.Image.SHA256 = strings.Repeat("0", 64)
	}
	if err := chk.Validate(); err != nil {
		return fmt.Errorf("measurement record: %w", err)
	}
	fmt.Printf("record %s %s %s · arch: %s\n", rec.Service, rec.Version, rec.Platform, rec.Arch)
	res, err := tr.GitTransport{WorkDir: *work}.Send(context.Background(), rec, *repo)
	if err != nil {
		return err
	}
	fmt.Printf("sent %s → %s @ %s\n", res.Path, res.Repo, res.Commit)
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
	if !quiet {
		fmt.Printf("received %s document, %d bytes → %s (nonce → %s.nonce)\n", doc.Format, len(doc.Raw), *out, *out)
	}
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
	cli.Okln("\n✓ enclave image ready: ", built.EIF, "   measurements: ", built.Measurements)
	fmt.Printf("next:  tb auditor launch --eif %s --port %d\n", built.EIF, b.Port)
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
	cli.Okln("\n✓ enclave image ready: ", built.EIF, "   measurements: ", built.Measurements)
	fmt.Printf("next:  tb auditor launch --eif %s --port %d\n", built.EIF, b.Port)
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

	fmt.Println()
	t := cli.Start(fmt.Sprintf("Asking the new enclave at %s for an attestation, to be sure it answers", started.Address))
	nonce, doc, err := ra.TCPRequester{Timeout: 30 * time.Second}.RequestFresh(ctx, started.Address)
	if err != nil {
		t.Fail("no answer")
		return err
	}
	if err := doc.ExpectFormat(awsra.Format); err != nil {
		t.Fail("wrong format")
		return err
	}
	selfDoc := filepath.Join(l.Out, "selfcheck.cbor")
	if err := os.WriteFile(selfDoc, doc.Raw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(selfDoc+".nonce", []byte(hex.EncodeToString(nonce)+"\n"), 0o644); err != nil {
		return err
	}
	t.Done(fmt.Sprintf("%s document, %d bytes → %s", doc.Format, len(doc.Raw), selfDoc))

	fmt.Printf("Security group: inbound TCP %d\n", l.Port)
	cmd, group, perr := launch.OpenPort(ctx, l.Port, os.Stdout)
	if perr == nil {
		cli.Okln("✓ enclave is up and reachable. anyone can now:  tb user verify --tee <this instance's public IP>:", l.Port, " --rpc … --contract …")
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
