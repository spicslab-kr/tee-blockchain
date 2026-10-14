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

	reg "attestamp/internal/chain/register/Vendor"
	"attestamp/internal/cli"
	tr "attestamp/internal/core/transfer/Vendor"
	toolchain "attestamp/internal/setup/build"
	build "attestamp/internal/setup/build/Vendor/aws"
	launch "attestamp/internal/setup/launch/Vendor/aws"
	parse "attestamp/internal/tee/parse/Vendor"
	ra "attestamp/internal/tee/request/Vendor"
	awsra "attestamp/internal/tee/request/Vendor/aws"
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
	case "version":
		fmt.Println("att-vendor ·", toolchain.ToolchainLine())
		err = toolchain.CheckToolchain()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		cli.Errorln("att-vendor: ", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `att-vendor

  version          print the pinned Go toolchain (internal/setup/build/GO_TOOLCHAIN) and the one this binary was built with;
                   exit 1 if they differ — build/pack refuse to run in that case, since the responder's bytes and the measurements depend on it
  registry-deploy  --rpc <url> --deployer-key-file <file> --approver <0x…>
                   deploy the Registry contract. one approver per registry, fixed at deploy time. the deployer key only pays gas
  register         --rpc <url> --registry <0x…> --signing-key-file <file> --measurements <measurements.json | NAME=hex,…>
                   (--config <build-config.json> | --service <name> --version <v> [--arch amd64|arm64]) [--platform aws-nitro] [--expires 2160h]
                   sign the measurements with my signing key and register them on the registry. the record key in the output is the lookup coordinate.
                   --config takes service, version and the build env from my build-config.json; the flags override it, with a warning
  lookup           --rpc <url> --registry <0x…> (--record-key <0x…> | --measurements <measurements.json | NAME=hex,…>)
                   read one record key directly, or find the record key(s) for the measurements first and read them all
  compare          --attested <measurements.json | NAME=hex,…> --expected <same shape>
                   match two sets of measurements as they are (parsed vs built, etc.). prints PASS/FAIL with the reasons
  compare          --attested <measurements.json | NAME=hex,…> --rpc <url> --registry <0x…> --record-key <0x…> [--trusted-approvers <0x…,0x…>]
                   read the registration from the chain and check the value, expiry and approver together. exit 1 on failure
  self-register    [--tee-addr <my enclave host:port>] [--build-measurements <file>] --rpc <url> --registry <0x…> --signing-key-file <file>
                   (--config <build-config.json> | --service <name> --version <v> [--arch amd64|arm64])
                   no Auditor: request and verify the attestation document from my own enclave, register the value under my signing key, read it back and
                   check it. with --build-measurements too, registers only if the attested value matches it. --config takes
                   service, version and the build env from my build-config.json; the flags override it, with a warning. --nonce
                   pins the nonce. with --service/--version empty and no --config, takes them from an existing registration on
                   this registry
  send             --repo <url> --config <build-config.json> [--workdir <clone>]
                   commit and push the build config to the source repository
  receive          --repo <url> [--ref <commit>] --into <dir>
                   clone the measurement record and image, and verify them
  build       --app-dir <dir with the software's Dockerfile>  [--image-name <n>] [--port 5000] [--out-dir out] [--responder <att-ra-responder>] [--verify]
              on the Nitro parent instance. builds the software image, layers the RA responder on top, and writes the EIF and measurements (PCR0·1·2).
              for source that the Auditor rebuilds
  pack        --docker-image <docker image you built>  [--image-name <n>] [--port 5000] [--out-dir out] [--responder <att-ra-responder>]
              same as build minus the docker build: takes your image, layers the responder, writes the EIF and measurements.
              for an image you hand to the Auditor as a file
  launch      --eif <file>  [--build-measurements <file>] [--enclave-name <n>] [--port 5000] [--memory <MiB>] [--cpus 2] [--cid 16] [--listen 0.0.0.0] [--out-dir out]
              on the Nitro parent instance. runs the EIF as an enclave, bridges TCP→vsock,
              requests RA from itself to confirm, then opens the security-group port (when permitted).
              --build-measurements is for a received EIF: the file it came with. the EIF is then measured itself (→ out/measurements.json) and launched only if it measures to that
  ra-serve    --listen tcp:<host:port> | vsock:<port>  --extractor nsm | fixture=<doc>  [--user-data <hex>]
              runs inside the TEE. takes a nonce per request and returns an RA document. a fixture is labelled aws-nitro
  lo-up       brings up loopback inside the enclave. once, before starting the software
  ra-proxy    --listen <host:port>  --vsock <cid>:<port>
              runs on the Nitro parent instance. forwards incoming TCP connections to the enclave's vsock
  ra-parse    --doc <file> [--nonce <hex|file>] [--root <pem>] [--format aws-nitro] [--out-file <file>] [--at <RFC3339>]
              verifies the document's signature, certificate chain and nonce, and extracts the measurements (PCR). the output goes straight into register --measurements
  ra-request  --tee-addr <host:port> [--out-file <file>] [--format <name>] [--timeout 30s]
              generates a nonce, sends it, and saves the RA document. the nonce goes to <file>.nonce.
              with --format, accepts only a document of that format. only aws-nitro documents can be parsed and verified; gcp-cs-jwt and azure-maa-jwt exist only on the request side

to do it by hand: commit build-config.json at the repository root and push; the other side clones it.`)
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
	// the build command compiles att-ra-responder in the current environment, so that is the build_env
	// the Auditor rebuilds to. a config that already declares one is kept as written.
	if cfg.BuildEnv.IsZero() {
		cfg.BuildEnv = tr.CurrentBuildEnv()
	}
	res, err := tr.GitTransport{WorkDir: *work}.Send(context.Background(), cfg, *repo)
	if err != nil {
		return err
	}
	fmt.Printf("sent %s → %s @ %s\n", res.Path, res.Repo, res.Commit)
	fmt.Printf("build env: %s\n", cfg.BuildEnv)
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
	fmt.Printf("%s %s %s · build env: %s · %d measurements · %s %s\n", got.Value.Service, got.Value.Version, got.Value.Platform,
		got.Value.BuildEnv, len(got.Value.Values), got.Value.BuildTool.Name, got.Value.BuildTool.Version)
	warnToolchain(got.Value.BuildEnv)
	return nil
}

func raRequest(args []string) error {
	fs := flag.NewFlagSet("ra-request", flag.ContinueOnError)
	teeAddr := fs.String("tee-addr", "", "TEE address host:port")
	outFile := fs.String("out-file", "ra-doc.cbor", "file to save the document to")
	format := fs.String("format", "", "expected document format (only aws-nitro can be verified). empty accepts any")
	timeout := fs.Duration("timeout", 30*time.Second, "RA request timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *teeAddr == "" {
		return errors.New("--tee-addr is required")
	}
	nonce, doc, err := ra.TCPRequester{Timeout: *timeout}.RequestFresh(context.Background(), *teeAddr)
	if err != nil {
		return err
	}
	if err := doc.ExpectFormat(*format); err != nil {
		return err
	}
	if err := os.WriteFile(*outFile, doc.Raw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(*outFile+".nonce", []byte(hex.EncodeToString(nonce)+"\n"), 0o644); err != nil {
		return err
	}
	if !quiet {
		fmt.Printf("received %s document, %d bytes → %s (nonce → %s.nonce)\n", doc.Format, len(doc.Raw), *outFile, *outFile)
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
	fs.StringVar(&b.ImageName, "image-name", "", "image and EIF name (default: the directory name)")
	fs.IntVar(&b.Port, "port", 5000, "port the responder listens on inside the enclave")
	fs.StringVar(&b.OutDir, "out-dir", "out", "output directory")
	fs.StringVar(&b.ResponderPath, "responder", "", "path to the responder (att-ra-responder). default: att-ra-responder next to this executable")
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
	fmt.Printf("next:  att-vendor launch --eif %s --port %d\n", built.EIF, b.Port)
	return nil
}

func packCmd(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	b := build.Build{}
	fs.StringVar(&b.DockerImage, "docker-image", "", "a docker image you built (name:tag)")
	fs.StringVar(&b.ImageName, "image-name", "", "EIF name (default: the docker image name without its tag)")
	fs.IntVar(&b.Port, "port", 5000, "port the responder listens on inside the enclave")
	fs.StringVar(&b.OutDir, "out-dir", "out", "output directory")
	fs.StringVar(&b.ResponderPath, "responder", "", "path to the responder (att-ra-responder). default: att-ra-responder next to this executable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if b.DockerImage == "" {
		return errors.New("--docker-image is required")
	}
	built, err := b.Run(context.Background(), os.Stdout)
	if err != nil {
		return err
	}
	cli.Okln("\n✓ enclave image ready: ", built.EIF, "   measurements: ", built.Measurements, "   — hand both to the Auditor")
	fmt.Printf("next:  att-vendor launch --eif %s --port %d\n", built.EIF, b.Port)
	return nil
}

func launchCmd(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	l := launch.Launch{}
	fs.StringVar(&l.EIF, "eif", "", "EIF file to run")
	fs.StringVar(&l.EnclaveName, "enclave-name", "", "enclave name (default: the EIF file name)")
	fs.IntVar(&l.Port, "port", 5000, "RA port. must match the value given to build or pack")
	fs.IntVar(&l.Memory, "memory", 0, "enclave memory in MiB (0: sized from the EIF)")
	fs.IntVar(&l.CPUs, "cpus", 2, "enclave CPU count")
	fs.IntVar(&l.CID, "cid", 16, "enclave vsock CID")
	fs.StringVar(&l.Listen, "listen", "0.0.0.0", "address the bridge listens on")
	fs.StringVar(&l.OutDir, "out-dir", "out", "directory for logs and the self-check document")
	fs.StringVar(&l.BuildMeasurements, "build-measurements", "", "the measurements.json that came with a received EIF: the EIF is measured itself and must match, or it is not launched")
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
	selfDoc := filepath.Join(l.OutDir, "selfcheck.cbor")
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
		cli.Okln("✓ enclave is up and reachable. anyone can now:  att-user verify --tee-addr <this instance's public IP>:", l.Port, " --rpc … --registry …")
		return nil
	}
	// Could not open it here: say what to open, where and how. The permission-denied case and
	// other failures (metadata, aws CLI) are told apart only on the second line.
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
Then anyone can:  att-user ra-request --tee-addr <this instance's public IP>:%d --format aws-nitro
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
