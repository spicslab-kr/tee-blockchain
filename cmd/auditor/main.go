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
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	reg "attestamp/internal/chain/register/auditor"
	"attestamp/internal/cli"
	tr "attestamp/internal/core/transfer/auditor"
	toolchain "attestamp/internal/setup/build"
	build "attestamp/internal/setup/build/auditor/aws"
	launch "attestamp/internal/setup/launch/auditor/aws"
	parse "attestamp/internal/tee/parse/auditor"
	ra "attestamp/internal/tee/request/auditor"
	awsra "attestamp/internal/tee/request/auditor/aws"
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
		fmt.Println("att-auditor ·", toolchain.ToolchainLine())
		err = toolchain.CheckToolchain()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		cli.Errorln("att-auditor: ", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `att-auditor

  version          print the pinned Go toolchain (internal/setup/build/GO_TOOLCHAIN) and the one this binary was built with;
                   exit 1 if they differ — build/pack refuse to run in that case, since the responder's bytes and the measurements depend on it
  registry-deploy  --rpc <url> --key-file <file> --approver <0x…>
                   deploy the Registry contract. one signer per contract, fixed at deploy time
  register         --rpc <url> --contract <0x…> --key-file <file> --measurements <measurements.json | NAME=hex,…>
                   (--config <build-config.json> | --service <name> --version <v> [--arch amd64|arm64]) [--platform aws-nitro] [--expires 2160h]
                   sign the measurements with my key and register them on the contract. the key in the output is the lookup coordinate.
                   --config takes service, version and the build env from the Vendor's build-config.json; the flags override it, with a warning
  lookup           --rpc <url> --contract <0x…> (--key <0x…> | --measurements <measurements.json | NAME=hex,…>)
                   read one key directly, or find the key(s) for the measurements first and read them all
  compare          --a <measurements.json | NAME=hex,…> --b <same shape>
                   match two sets of measurements as they are (parsed vs built, etc.). prints PASS/FAIL with the reasons
  compare          --a <measurements.json | NAME=hex,…> --rpc <url> --contract <0x…> --key <0x…> [--trusted <0x…,0x…>]
                   read the registration from the chain and check the value, expiry and trusted signer together. exit 1 on failure
  audit            --tee <my enclave host:port> --measurements <my out/measurements.json> --rpc <url> --contract <0x…> --key-file <file>
                   (--config <build-config.json> | --service <name> --version <v> [--arch amd64|arm64]) [--vendor-tee <the Vendor's enclave host:port>]
                   in one go: request and verify the attestation document from my own enclave, check the attested value against --measurements (the value I
                   built or measured myself), register that value only if they match, then read it back and check it.
                   --config takes service, version and the build env from the Vendor's build-config.json (receive put it in the
                   clone root); the flags override it, with a warning. --nonce pins the nonce. with --service/--version empty and no
                   --config, takes them from an existing registration of the same measurements on this contract. without --tee
                   registers --measurements as is (no audit).
                   --vendor-tee also asks the Vendor's running enclave and says whether it runs the same value; what is
                   registered is still my value
  receive          --repo <url> [--ref <commit>] --into <dir> [--ignore-env]
                   clone the source repository and read the build config. build from that directory
  send             --repo <url> --config <build-config.json> --measurements <out/measurements.json> --image <file> [--build-tool-version <v>] [--workdir <clone>]
                   build the measurement record from the Vendor's build-config.json (service, version, build_env) and my
                   measurements.json (PCR0·1·2), write it next to the measurements, then commit and push it with the image.
                   --build-tool-version is nitro-cli's; without it, what nitro-cli --version says here
  send             --repo <url> --record <measurement-record.json> [--image <file>] [--workdir <clone>]
                   the same with a hand-written record. it must say which build environment it is for ("build_env": os,
                   arch amd64 | arm64, go, trimpath) — measurements differ per architecture and per Go toolchain
  build       --app-dir <dir with the software's Dockerfile>  [--name <n>] [--port 5000] [--out out] [--responder <att-ra-responder>] [--verify]
              on the Nitro parent instance. builds the software image, layers the RA responder on top, and writes the EIF and measurements (PCR0·1·2).
              for source that the Auditor rebuilds
  pack        --image <docker image you built>  [--name <n>] [--port 5000] [--out out] [--responder <att-ra-responder>]
              same as build minus the docker build: takes your image, layers the responder, writes the EIF and measurements.
              for an image handed over as a file
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

to do it by hand: commit build-config.json at the repository root and push; the other side clones it.`)
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
	// the config carries service, version and build_env, so audit needs none of them typed again
	fmt.Printf("next: att-auditor audit --config %s --tee <this-instance>:5000 --measurements out/measurements.json --rpc <url> --contract <0x…> --key-file <key>\n", got.Path)
	return nil
}

func send(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	repo := fs.String("repo", "", "repository URL to push to")
	recPath := fs.String("record", "", "path to a hand-written measurement-record.json. instead of it, --config and --measurements build the record here")
	config := fs.String("config", "", "the Vendor's build-config.json: the record's service, version, platform and build_env are taken from it")
	measurements := fs.String("measurements", "", "my out/measurements.json (nitro-cli's {\"Measurements\":{\"PCR0\":…}}): the record's values are taken from it")
	image := fs.String("image", "", "image file (aws: the EIF). overrides image.path in the record; required with --config")
	buildToolVersion := fs.String("build-tool-version", "", "with --config: nitro-cli's version, for the record's build_tool. default: what `nitro-cli --version` says here")
	work := fs.String("workdir", "", "an existing clone to work in (default: a temporary clone)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || (*recPath == "" && (*config == "" || *measurements == "")) {
		return errors.New("--repo and either --record, or --config with --measurements (the record is built from them), are required")
	}
	var rec tr.MeasurementRecord
	if *recPath != "" {
		b, err := os.ReadFile(*recPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &rec); err != nil {
			return fmt.Errorf("measurement record: %w", err)
		}
	} else {
		var err error
		if rec, err = buildRecord(*config, *measurements, *image, *buildToolVersion); err != nil {
			return err
		}
	}
	if *image != "" {
		rec.Image.Path = *image
	}
	// Screen the record before cloning and hashing the image (Send checks again). Send fills in the image hash, so only that slot is filled in for this check.
	// A record without build_env cannot say which architecture or toolchain it is for, so it is not sent.
	chk := rec
	if chk.Image.Path != "" && chk.Image.SHA256 == "" {
		chk.Image.SHA256 = strings.Repeat("0", 64)
	}
	if err := chk.Validate(); err != nil {
		return fmt.Errorf("measurement record: %w", err)
	}
	fmt.Printf("record %s %s %s · build env: %s\n", rec.Service, rec.Version, rec.Platform, rec.BuildEnv)
	warnToolchain(rec.BuildEnv)
	if *recPath == "" {
		// the record was built here — leave it on disk next to the measurements too, with the image's sha256 filled in,
		// so the Vendor-side file exists as it does after a hand-written send
		written, err := writeRecordBeside(rec, *measurements)
		if err != nil {
			return err
		}
		fmt.Printf("record written to %s\n", written)
	}
	res, err := tr.GitTransport{WorkDir: *work}.Send(context.Background(), rec, *repo)
	if err != nil {
		return err
	}
	fmt.Printf("sent %s → %s @ %s\n", res.Path, res.Repo, res.Commit)
	return nil
}

// buildRecord — send --config --measurements: the measurement record from what is already on disk. service, version,
// platform and build_env are the Vendor's build-config.json (the build that was measured); values are nitro-cli's
// measurements.json (PCR0·1·2); image.path is --image; build_tool is nitro-cli with --build-tool-version, or the
// version `nitro-cli --version` reports on this instance. Send fills in image.sha256.
func buildRecord(configPath, measPath, image, toolVersion string) (tr.MeasurementRecord, error) {
	cfg, err := loadBuildConfig(configPath)
	if err != nil {
		return tr.MeasurementRecord{}, err
	}
	if image == "" {
		return tr.MeasurementRecord{}, errors.New("--image is required with --config: the record names the EIF that was measured")
	}
	values, err := readMeasurements(measPath)
	if err != nil {
		return tr.MeasurementRecord{}, err
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	if toolVersion == "" {
		toolVersion = nitroCLIVersion()
	}
	if toolVersion == "" {
		return tr.MeasurementRecord{}, errors.New("--build-tool-version is required: nitro-cli is not on PATH here, so give the version of the nitro-cli that built the image (nitro-cli --version on the instance)")
	}
	return tr.MeasurementRecord{
		Schema: tr.SchemaMeasurementRecord, Service: cfg.Service, Version: cfg.Version, Platform: cfg.Platform,
		BuildEnv:  cfg.BuildEnv,
		Image:     tr.Image{Path: image},
		Values:    values,
		BuildTool: tr.BuildTool{Name: "nitro-cli", Version: toolVersion},
	}, nil
}

// nitroCLIVersion — the version number in `nitro-cli --version` ("Amazon Web Services Nitro CLI 1.3.0"). Empty if
// nitro-cli is not here or says nothing that looks like a version.
func nitroCLIVersion() string {
	out, err := exec.Command("nitro-cli", "--version").Output()
	if err != nil {
		return ""
	}
	for _, f := range strings.Fields(string(out)) {
		f = strings.TrimPrefix(f, "v")
		if len(f) > 0 && f[0] >= '0' && f[0] <= '9' && strings.Contains(f, ".") {
			return f
		}
	}
	return ""
}

// writeRecordBeside — the built record as a file next to measurements.json (or in the working directory when
// --measurements was NAME=hex,…), with image.sha256 filled and image.path as the bare file name, as Send pushes it.
func writeRecordBeside(rec tr.MeasurementRecord, measPath string) (string, error) {
	if rec.Image.Path != "" {
		sum, err := tr.FileDigest(rec.Image.Path)
		if err != nil {
			return "", fmt.Errorf("image file: %w", err)
		}
		rec.Image.SHA256 = sum
		rec.Image.Path = filepath.Base(rec.Image.Path)
	}
	b, err := tr.Encode(rec)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(filepath.Dir(measPath), tr.MeasurementRecordFile)
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return "", err
	}
	return dst, nil
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
	fs.StringVar(&b.Binary, "responder", "", "path to the responder (att-ra-responder). default: att-ra-responder next to this executable")
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
	fmt.Printf("next:  att-auditor launch --eif %s --port %d\n", built.EIF, b.Port)
	return nil
}

func packCmd(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	b := build.Build{}
	fs.StringVar(&b.Image, "image", "", "a docker image you built (name:tag)")
	fs.StringVar(&b.Name, "name", "", "EIF name (default: the image name without its tag)")
	fs.IntVar(&b.Port, "port", 5000, "port the responder listens on inside the enclave")
	fs.StringVar(&b.Out, "out", "out", "output directory")
	fs.StringVar(&b.Binary, "responder", "", "path to the responder (att-ra-responder). default: att-ra-responder next to this executable")
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
	fmt.Printf("next:  att-auditor launch --eif %s --port %d\n", built.EIF, b.Port)
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
		cli.Okln("✓ enclave is up and reachable. anyone can now:  att-user verify --tee <this instance's public IP>:", l.Port, " --rpc … --contract …")
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
Then anyone can:  att-user ra-request --tee <this instance's public IP>:%d --format aws-nitro
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
