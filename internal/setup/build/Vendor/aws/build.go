// Package aws builds a software Dockerfile into an enclave image (EIF) and its measurements (PCR0, PCR1, PCR2) for AWS Nitro.
// Result holds the values to write into the measurement record. The tools (docker, nitro-cli) are run through exec, not linked as libraries.
//
// One step goes in here beyond building the software: layering in the attestation responder. A Nitro enclave can open
// /dev/nsm only from inside and has no network, so for a third party to request attestation the responder has to be
// inside the image. The responder's code lives in internal/tee/request/Vendor/aws (Extractor, vsock), and its ra-serve command runs from the image's entrypoint.
// The build adds the responder binary, an entrypoint script and the software's original command on top of the software image, and leaves the software's own files as they are.
package aws

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"attestamp/internal/cli"
	"attestamp/internal/setup/build"
)

//go:embed entrypoint.sh
var entrypointScript []byte

// responderBinaryName is the file name cmd/ra-responder produces. Every role looks for a file with exactly this
// name next to its executable, so that builds by different roles put the same bytes in the image.
const responderBinaryName = "att-ra-responder"

// Build is the input to one build. The Dockerfile in AppDir is the software.
type Build struct {
	AppDir      string // directory holding the software's Dockerfile (build). Set exactly one of AppDir and DockerImage
	DockerImage string // a docker image the user already built (pack): the user runs docker build, att does the layering, normalizing and EIF
	ImageName   string // image and EIF name. Defaults to the directory name (pack: the image name before the tag)
	Port        int    // vsock port the responder listens on inside the enclave. 0 means 5000

	// ResponderPath is the responder to layer in. If empty, att-ra-responder next to the executable is used.
	//
	// Every role layers in the identical file built from cmd/ra-responder, so the Vendor's and the Auditor's
	// images can be equal. Layering att-vendor or att-auditor themselves would put a different /att-ra binary
	// in each role's image and give different PCR0 and PCR2 (measured on two EC2 instances).
	ResponderPath string
	OutDir        string // result directory. Defaults to out
	Verify        bool   // build the same Dockerfile once more without cache and check that the PCRs match (verify.go)
}

// Result is what goes into the measurement record.
type Result struct {
	Name         string
	Image        string              // docker image tag with the responder layered in
	EIF          string              // enclave image file
	Measurements string              // raw nitro-cli build-enclave output (json)
	PCR          map[string]string   // PCR0, PCR1, PCR2
	Responder    build.ResponderInfo // build info and sha256 of the layered-in att-ra-responder, kept in the record so the other side can compare
}

var ErrNotLinux = errors.New("build runs on the Nitro parent instance (linux)")

// Run goes through the build steps: tools, software image, responder layering, normalizing, EIF and PCRs.
//
// There are two entry points. AppDir (build): att starts from docker build, so the Vendor and the Auditor running the
// same command get the same values. DockerImage (pack): the user hands over an image built their own way and att only layers,
// normalizes and makes the EIF; the Auditor measures the received EIF file, so only the Vendor needs to build it.
func (b Build) Run(ctx context.Context, log io.Writer) (Result, error) {
	var res Result
	if runtime.GOOS != "linux" {
		return res, ErrNotLinux
	}
	if err := build.CheckToolchain(); err != nil {
		return res, err
	}
	if (b.AppDir == "") == (b.DockerImage == "") {
		return res, errors.New("give either an app dir (build) or an image (pack)")
	}
	if b.DockerImage != "" && b.Verify {
		return res, errors.New("--verify needs the Dockerfile: it rebuilds; pack only packs an image you built")
	}
	var appDir, dockerfile string
	if b.DockerImage == "" {
		var err error
		if appDir, err = filepath.Abs(b.AppDir); err != nil {
			return res, err
		}
		dockerfile = filepath.Join(appDir, "Dockerfile")
		if st, err := os.Stat(dockerfile); err != nil || st.IsDir() {
			return res, fmt.Errorf("%s: Dockerfile not found", appDir)
		}
		if b.ImageName == "" {
			b.ImageName = Sanitize(filepath.Base(appDir))
		}
	} else if b.ImageName == "" {
		name, _, _ := strings.Cut(b.DockerImage, ":")
		b.ImageName = Sanitize(filepath.Base(name))
	}
	if b.Port == 0 {
		b.Port = 5000
	}
	if b.OutDir == "" {
		b.OutDir = "out"
	}
	if b.ResponderPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return res, err
		}
		b.ResponderPath = filepath.Join(filepath.Dir(exe), responderBinaryName)
	}
	if _, err := os.Stat(b.ResponderPath); err != nil {
		return res, fmt.Errorf("%s: attestation responder not found — build cmd/ra-responder and put it there, or point ResponderPath at it", b.ResponderPath)
	}
	// A file being there is not enough. What gets layered in and measured is this file's bytes, and those are fixed by
	// the Go version, -trimpath and GOOS/GOARCH the responder was built with (CheckToolchain covers the att binary only).
	// Read the build info Go left in the binary and refuse a responder that cannot produce the same bytes as the other side.
	responder, err := build.ReadResponder(b.ResponderPath)
	if err != nil {
		return res, err
	}
	if err := build.CheckResponder(responder, runtime.GOARCH); err != nil {
		return res, err
	}
	res.Responder = responder
	fmt.Fprintln(log, responder.String())
	if err := os.MkdirAll(b.OutDir, 0o755); err != nil {
		return res, err
	}
	res.Name = b.ImageName
	r := runner{log: log, sudo: os.Geteuid() != 0}

	// Step labels: build has 5 steps, pack has 4 because it skips docker build.
	verb, total, n := "build", 5, 0
	if b.DockerImage != "" {
		verb, total = "pack", 4
	}
	// Two lines per step: what is being done, then one sentence on why, so a reader who does not know our terms can follow.
	step := func(what, why string) {
		n++
		fmt.Fprintf(log, "\n%s %s…\n           %s\n", cli.Green(fmt.Sprintf("[%s %d/%d]", verb, n, total)), what, why)
	}

	if b.DockerImage == "" {
		risks, err := LintDockerfile(dockerfile)
		if err != nil {
			return res, err
		}
		printRisks(log, dockerfile, risks)
	}

	step("Installing the tools (nitro-cli, docker)", "docker builds the image; nitro-cli turns it into an enclave image and measures it")
	if err := r.run(ctx, "dnf", "install", "-y", "-q", "aws-nitro-enclaves-cli", "aws-nitro-enclaves-cli-devel", "docker"); err != nil {
		return res, err
	}
	if err := r.run(ctx, "systemctl", "enable", "--now", "docker"); err != nil {
		return res, err
	}

	appImage := b.DockerImage
	res.Image = b.ImageName + ":att"
	if b.DockerImage == "" {
		appImage = b.ImageName + ":app"
		step(fmt.Sprintf("Building the software image from %s → %s", appDir, appImage), "the software exactly as its own Dockerfile makes it — nothing of ours inside yet. an Auditor runs this same step on the same source")
		if err := r.run(ctx, "docker", "build", "-q", "-t", appImage, appDir); err != nil {
			return res, err
		}
	} else if _, err := r.output(ctx, "docker", "image", "inspect", appImage); err != nil {
		return res, fmt.Errorf("%s: no such docker image here — docker build it first, or docker load it", appImage)
	}

	step(fmt.Sprintf("Adding the attestation responder to the image: %s → %s", appImage, res.Image), "a small program that runs inside the enclave next to the software and hands out signed attestation documents on request, which lets anyone ask the enclave what it is running")
	appCmd, err := r.imageCommand(ctx, appImage)
	if err != nil {
		return res, err
	}
	fmt.Fprintf(log, "software command: %s\n", strings.Join(appCmd, " "))
	fmt.Fprintln(log, build.ToolchainLine())
	if err := b.layerResponder(ctx, r, appImage, res.Image, appCmd); err != nil {
		return res, err
	}

	step("Repacking the image so that anyone rebuilding it gets the same bytes", "docker layers carry build timestamps and an arbitrary file order; we lay the filesystem out in a fixed order and drop files that only record when it was built, so the Vendor's and the Auditor's builds of one source are byte-identical")
	first, err := normalize(ctx, r, res.Image, b.OutDir)
	if err != nil {
		return res, err
	}

	res.EIF = filepath.Join(b.OutDir, b.ImageName+".eif")
	res.Measurements = filepath.Join(b.OutDir, "measurements.json")
	step("Making the enclave image (EIF) → "+res.EIF, "nitro-cli produces the bootable enclave image and its measurements PCR0·1·2 — the fingerprint that the enclave will attest to and that goes on the chain")
	out, err := r.output(ctx, "nitro-cli", "build-enclave", "--docker-uri", res.Image, "--output-file", res.EIF)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(res.Measurements, out, 0o644); err != nil {
		return res, err
	}
	if res.PCR, err = ReadPCR(out); err != nil {
		return res, err
	}
	fmt.Fprintln(log, cli.Green("✓ measured"), "— these three values identify the image:")
	for _, k := range []string{"PCR0", "PCR1", "PCR2"} {
		fmt.Fprintf(log, "    %s %s\n", k, res.PCR[k])
	}
	if b.Verify {
		if err := b.verify(ctx, r, appDir, appImage, first, res.PCR); err != nil {
			return res, err
		}
	}
	return res, nil
}

// layerResponder layers the responder binary and the entrypoint on top of the software image. The original command is passed as a file,
// because nitro-cli drops a Docker CMD behind a script entrypoint.
func (b Build) layerResponder(ctx context.Context, r runner, from, to string, appCmd []string) error {
	dir, err := os.MkdirTemp("", "att-ra-image")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bin, err := os.ReadFile(b.ResponderPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "att-ra"), bin, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "entrypoint.sh"), entrypointScript, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "att-app-cmd"), []byte(strings.Join(appCmd, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	dockerfile := fmt.Sprintf(`FROM %s
COPY att-ra /att-ra
COPY entrypoint.sh /att-entrypoint.sh
COPY att-app-cmd /att-app-cmd
ENV ATT_RA_PORT=%d
ENTRYPOINT ["/att-entrypoint.sh"]
`, from, b.Port)
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}
	return r.run(ctx, "docker", "build", "-q", "-t", to, dir)
}

// normalize throws away the layers docker build produced and repacks the files itself, in name order.
//
// docker build decides file order and timestamps inside layers anew on every run. Even when the final file
// contents are identical (measured: every sha256 matches), the bytes of the layer tar holding them differ, so when the
// Vendor and the Auditor build the same source independently, PCR0 and PCR2 diverge (reproduced on two different
// physical instances). Exporting drops the layer boundaries and leaves just the files; repacking those with name-sorted
// order and fixed times (tar --sort=name --mtime=@0 --owner=0 --group=0) and importing under the same tag gives
// exactly matching PCR0, PCR1 and PCR2 even when built independently on different physical servers at different times
// (confirmed by measurement).
//
// import drops the image config (ENTRYPOINT, ENV), so the original is put back. That is measured too: changing only
// the ENTRYPOINT, with the files untouched, was measured to change PCR0 and PCR2.
//
// workDir must be on real disk (b.OutDir). The default OS temp directory (/tmp) is a RAM-backed tmpfs of only a few GiB on
// Amazon Linux, and the exported tar, the extracted files and the repacked tar together can exceed that
// (reproduced: "wrote only N of 10240 bytes").
func normalize(ctx context.Context, r runner, image, workDir string) (manifest, error) {
	out, err := r.output(ctx, "docker", "image", "inspect", image)
	if err != nil {
		return nil, err
	}
	var v []struct {
		Config struct {
			Env        []string
			Entrypoint []string
			Cmd        []string
		}
	}
	if err := json.Unmarshal(out, &v); err != nil || len(v) != 1 {
		return nil, fmt.Errorf("docker inspect %s: unexpected output", image)
	}
	cfg := v[0].Config

	dir, err := os.MkdirTemp(workDir, "att-canon-*")
	if err != nil {
		return nil, err
	}
	// The tree tar extracted with sudo is owned by root and os.RemoveAll cannot delete it; left alone it would pile up in out/ on every build.
	defer func() {
		if os.RemoveAll(dir) != nil {
			r.cmd(ctx, "rm", "-rf", dir).Run()
		}
	}()

	container := fmt.Sprintf("att-canon-%d", os.Getpid())
	r.cmd(ctx, "docker", "rm", "-f", container).Run() // leftover from an earlier failed run, if any
	if err := r.run(ctx, "docker", "create", "--name", container, image); err != nil {
		return nil, err
	}
	defer r.cmd(ctx, "docker", "rm", container).Run()

	rootfs := filepath.Join(dir, "rootfs.tar")
	if err := r.run(ctx, "docker", "export", container, "-o", rootfs); err != nil {
		return nil, err
	}
	extract := filepath.Join(dir, "extract")
	if err := os.Mkdir(extract, 0o755); err != nil {
		return nil, err
	}
	if err := r.run(ctx, "tar", "-xf", rootfs, "-C", extract); err != nil {
		return nil, err
	}
	os.Remove(rootfs) // fully extracted, no longer needed; lowers peak disk use while repacking
	for _, d := range scrubBuildScratch(ctx, r, extract) {
		fmt.Fprintf(r.log, "    dropped build scratch: /%s\n", d)
	}
	for _, p := range fixApkVirtualVersions(ctx, r, extract) {
		fmt.Fprintf(r.log, "    pinned apk virtual-package versions (they were the install time): /%s\n", p)
	}
	for _, p := range stripBuildLogs(ctx, r, extract) {
		fmt.Fprintf(r.log, "    dropped a log stamped with the build time: /%s\n", p)
	}
	for _, p := range stripBuildCaches(ctx, r, extract) {
		fmt.Fprintf(r.log, "    dropped a build-time cache (it records when, not what): /%s\n", p)
	}

	m, err := readManifest(ctx, r, extract)
	if err != nil {
		return nil, err
	}

	canon := filepath.Join(dir, "canon.tar")
	if err := r.run(ctx, "tar", "--sort=name", "--mtime=@0", "--owner=0", "--group=0", "--numeric-owner",
		"-cf", canon, "-C", extract, "."); err != nil {
		return nil, err
	}

	args := []string{"import"}
	for _, e := range cfg.Env {
		args = append(args, "--change", envChange(e))
	}
	if len(cfg.Entrypoint) > 0 {
		b, _ := json.Marshal(cfg.Entrypoint)
		args = append(args, "--change", "ENTRYPOINT "+string(b))
	}
	if len(cfg.Cmd) > 0 {
		b, _ := json.Marshal(cfg.Cmd)
		args = append(args, "--change", "CMD "+string(b))
	}
	args = append(args, canon, image)
	if err := r.run(ctx, "docker", args...); err != nil {
		return nil, err
	}
	return m, nil
}

// buildLogs are logs a package manager writes during docker build. They record when something was installed, and say little about what
// (apk: "Running `apk add ...` at 2026-09-26 08:14:36"), so their content differs on every build of the same source. Measured:
// when the Vendor and the Auditor each built the official nginx alpine-slim Dockerfile, only /var/log/apk.log out of 725 files
// differed, and PCR0 and PCR2 diverged. They measure a time, not a file, so they are left out of the measurement. The software's own logs are not touched.
var buildLogs = []string{
	"var/log/apk.log",                                             // alpine
	"var/log/dpkg.log", "var/log/alternatives.log", "var/log/apt", // debian · ubuntu
	"var/log/yum.log", "var/log/dnf.log", "var/log/dnf.rpm.log", "var/log/dnf.librepo.log", "var/log/hawkey.log", // rhel · amazon
}

// stripBuildLogs removes buildLogs from the extracted rootfs and returns the relative paths it actually removed.
// The rootfs is root-owned because tar extracted it with sudo; if it cannot be removed directly, the runner (sudo rm) does it.
func stripBuildLogs(ctx context.Context, r runner, root string) []string {
	var removed []string
	for _, p := range buildLogs {
		full := filepath.Join(root, filepath.FromSlash(p))
		if !exists(ctx, r, full) {
			continue
		}
		if err := os.RemoveAll(full); err != nil {
			if err := r.cmd(ctx, "rm", "-rf", full).Run(); err != nil {
				fmt.Fprintf(r.log, "warning: could not remove %s — the build time stays in the measurements: %v\n", p, err)
				continue
			}
		}
		removed = append(removed, p)
	}
	return removed
}

// ReadPCR extracts the measurements from nitro-cli build-enclave output.
func ReadPCR(buildOutput []byte) (map[string]string, error) {
	var v struct {
		Measurements map[string]string
	}
	if err := json.Unmarshal(buildOutput, &v); err != nil {
		return nil, fmt.Errorf("build-enclave output: %w", err)
	}
	return v.Measurements, nil
}

// Sanitize cleans a directory name so it can be used as a docker tag and an enclave name.
func Sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		out = "app"
	}
	return out
}

// runner calls docker, nitro-cli and systemctl. It prefixes sudo when not root.
type runner struct {
	log  io.Writer
	sudo bool
}

func (r runner) cmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	if r.sudo {
		return exec.CommandContext(ctx, "sudo", append([]string{name}, args...)...)
	}
	return exec.CommandContext(ctx, name, args...)
}

func (r runner) run(ctx context.Context, name string, args ...string) error {
	c := r.cmd(ctx, name, args...)
	c.Stdout = r.log
	c.Stderr = r.log
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, args[0], err)
	}
	return nil
}

func (r runner) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	c := r.cmd(ctx, name, args...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %v\n%s", name, args[0], err, stderr.String())
	}
	return out, nil
}

// imageCommand returns the image ENTRYPOINT followed by CMD.
func (r runner) imageCommand(ctx context.Context, image string) ([]string, error) {
	out, err := r.output(ctx, "docker", "image", "inspect", image)
	if err != nil {
		return nil, err
	}
	var v []struct {
		Config struct {
			Entrypoint []string
			Cmd        []string
		}
	}
	if err := json.Unmarshal(out, &v); err != nil || len(v) != 1 {
		return nil, fmt.Errorf("docker inspect %s: unexpected output", image)
	}
	cmd := append(v[0].Config.Entrypoint, v[0].Config.Cmd...)
	if len(cmd) == 0 {
		return nil, fmt.Errorf("%s has no ENTRYPOINT or CMD; nothing to run", image)
	}
	return cmd, nil
}

// envChange is one ENV line for docker import --change. When the value contains spaces (measured: the official php image's
// PHPIZE_DEPS="autoconf dpkg-dev dpkg file g++ ..."), the unquoted `ENV k=v` form is rejected with "can't find = in dpkg-dev",
// so it always writes ENV k="v" and escapes backslashes and quotes.
func envChange(kv string) string {
	k, v, _ := strings.Cut(kv, "=")
	v = strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v)
	return `ENV ` + k + `="` + v + `"`
}
