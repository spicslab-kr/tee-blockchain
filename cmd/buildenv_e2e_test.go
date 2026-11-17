// build_env, end to end on the command line. att-vendor send writes its own build environment (OS, architecture, Go) into build-config.json,
// and att-auditor receive compares it with its own instance and refuses if they differ (with --ignore-env it only warns).
// On aws-nitro send also records the Amazon Linux release and the docker and nitro-cli versions, receive compares the
// release, and build --config the versions; what can be seen off an instance is tested here.
// The remote is a local bare repository and real git runs, the same way as the git tests under internal/core/transfer.
package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRemoteRepo seeds one bare repository with a main.go and returns its path and the seed commit.
func newRemoteRepo(t *testing.T) (remote, seedCommit string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	remote = filepath.Join(t.TempDir(), "remote.git")
	git(t, "", "init", "--quiet", "--bare", "--initial-branch=main", remote)
	seed := cloneRemote(t, remote)
	if err := os.WriteFile(filepath.Join(seed, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "add", "main.go")
	git(t, seed, "commit", "--quiet", "-m", "seed")
	git(t, seed, "push", "--quiet", "-u", "origin", "main")
	return remote, git(t, seed, "rev-parse", "HEAD")
}

func cloneRemote(t *testing.T, remote string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "work")
	git(t, "", "clone", "--quiet", remote, dir)
	git(t, dir, "config", "user.email", "t@t")
	git(t, dir, "config", "user.name", "t")
	return dir
}

// buildConfigJSON is the config to pass to send. build_env is left empty on purpose: the point is to see whether send fills it in.
func buildConfigJSON(t *testing.T, commit string, env map[string]any) []byte {
	t.Helper()
	m := map[string]any{
		"schema": "attestamp.build-config/1", "service": "svc", "version": "1.2.0", "platform": "aws-nitro",
		"source":   map[string]string{"repo": "https://example.com/svc.git", "commit": commit},
		"tee_info": map[string]string{"instance_id": "i-0abc", "region": "ap-northeast-2", "ami_id": "ami-0123"},
	}
	if env != nil {
		m["build_env"] = env
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// pushConfig imitates the Vendor: commits build-config.json to the remote and pushes it (bypassing send, with contents chosen directly).
func pushConfig(t *testing.T, remote string, cfg []byte) {
	t.Helper()
	work := cloneRemote(t, remote)
	if err := os.WriteFile(filepath.Join(work, "build-config.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", "build-config.json")
	git(t, work, "commit", "--quiet", "-m", "cfg")
	git(t, work, "push", "--quiet")
}

func runCmd(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("%s %v: %v", bin, args, err)
		}
		code = ee.ExitCode()
	}
	return o.String(), e.String(), code
}

func otherArch() string {
	if runtime.GOARCH == "amd64" {
		return "arm64"
	}
	return "amd64"
}

// att-vendor send records the OS/architecture/Go it runs on as build_env and pushes.
func TestBuildEnv_SendRecordsRunningEnv(t *testing.T) {
	dir := t.TempDir()
	vendor := build(t, dir, "./cmd/vendor")
	remote, _ := newRemoteRepo(t)
	cfgPath := filepath.Join(dir, "build-config.json")
	if err := os.WriteFile(cfgPath, buildConfigJSON(t, "", nil), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCmd(t, vendor, "send", "--repo", remote, "--config", cfgPath, "--workdir", cloneRemote(t, remote))
	if code != 0 {
		t.Fatalf("send exit %d\n%s%s", code, stdout, stderr)
	}
	wantLine := "build env: " + runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version()
	if !strings.Contains(stdout, wantLine) {
		t.Errorf("send must print the declared env %q, got:\n%s", wantLine, stdout)
	}

	// fetch it again from the remote and read it
	check := cloneRemote(t, remote)
	b, err := os.ReadFile(filepath.Join(check, "build-config.json"))
	if err != nil {
		t.Fatalf("build-config.json not pushed: %v", err)
	}
	var got struct {
		BuildEnv struct {
			OS       string `json:"os"`
			Arch     string `json:"arch"`
			Go       string `json:"go"`
			Trimpath bool   `json:"trimpath"`
		} `json:"build_env"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.BuildEnv.OS != runtime.GOOS || got.BuildEnv.Arch != runtime.GOARCH || got.BuildEnv.Go != runtime.Version() {
		t.Errorf("pushed build_env = %+v, want %s/%s %s", got.BuildEnv, runtime.GOOS, runtime.GOARCH, runtime.Version())
	}
}

// send then receive on the same machine: the receiver reports it matches its own environment and exits 0.
func TestBuildEnv_ReceiveMatchingEnvPasses(t *testing.T) {
	dir := t.TempDir()
	vendor := build(t, dir, "./cmd/vendor")
	auditor := build(t, dir, "./cmd/auditor")
	remote, _ := newRemoteRepo(t)
	cfgPath := filepath.Join(dir, "build-config.json")
	if err := os.WriteFile(cfgPath, buildConfigJSON(t, "", nil), 0o644); err != nil {
		t.Fatal(err)
	}
	if stdout, stderr, code := runCmd(t, vendor, "send", "--repo", remote, "--config", cfgPath, "--workdir", cloneRemote(t, remote)); code != 0 {
		t.Fatalf("send exit %d\n%s%s", code, stdout, stderr)
	}

	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code != 0 {
		t.Fatalf("receive exit %d\n%s%s", code, stdout, stderr)
	}
	env := runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version()
	for _, want := range []string{"build env: " + env, "(Vendor)", "(this instance)", "build env matches the Vendor's"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("receive stdout must contain %q, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stderr, "cannot produce") {
		t.Errorf("same env must not be refused:\n%s", stderr)
	}
}

// If the Vendor wrote a different architecture, receive refuses and names both architectures (exit != 0).
// With --ignore-env it prints the same message as a warning only and exits 0.
func TestBuildEnv_ReceiveRejectsOtherArch(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	pushConfig(t, remote, buildConfigJSON(t, seed, map[string]any{
		"os": runtime.GOOS, "arch": otherArch(), "go": runtime.Version(), "trimpath": true,
	}))

	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code == 0 {
		t.Fatalf("receive on the wrong architecture must fail\n%s%s", stdout, stderr)
	}
	for _, want := range []string{otherArch(), runtime.GOARCH, "cannot produce the Vendor's measurements", "--ignore-env"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr must name %q, got:\n%s", want, stderr)
		}
	}
	if !strings.Contains(stdout, "build env: "+runtime.GOOS+"/"+otherArch()) || !strings.Contains(stdout, "build env: "+runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("both envs must be printed before refusing, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "matches") {
		t.Errorf("must not claim a match:\n%s", stdout)
	}

	stdout, stderr, code = runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"), "--ignore-env")
	if code != 0 {
		t.Fatalf("--ignore-env must let it through, exit %d\n%s%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, otherArch()) {
		t.Errorf("--ignore-env must still warn naming the arch, got:\n%s", stderr)
	}
}

// A different Go version alone is refused: a changed toolchain changes the measurements.
func TestBuildEnv_ReceiveRejectsOtherGoVersion(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	pushConfig(t, remote, buildConfigJSON(t, seed, map[string]any{
		"os": runtime.GOOS, "arch": runtime.GOARCH, "go": "go1.0.0-not-this-one", "trimpath": true,
	}))
	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code == 0 {
		t.Fatalf("receive on another Go version must fail\n%s%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "go1.0.0-not-this-one") || !strings.Contains(stderr, runtime.Version()) {
		t.Errorf("stderr must name both Go versions, got:\n%s", stderr)
	}
}

// A Vendor build made without trimpath only gets a warning, not a refusal.
func TestBuildEnv_ReceiveWarnsOnNoTrimpath(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	pushConfig(t, remote, buildConfigJSON(t, seed, map[string]any{
		"os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(), "trimpath": false,
	}))
	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code != 0 {
		t.Fatalf("trimpath=false is a warning, not a refusal; exit %d\n%s%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "trimpath") {
		t.Errorf("want a trimpath warning on stderr, got:\n%s", stderr)
	}
}

// A build-config.json with no build_env is rejected for aws-nitro.
func TestBuildEnv_ReceiveRejectsConfigWithoutBuildEnv(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	pushConfig(t, remote, buildConfigJSON(t, seed, nil))
	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code == 0 {
		t.Fatalf("a config without build_env must be rejected\n%s%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "build_env") {
		t.Errorf("stderr must say what is missing, got:\n%s", stderr)
	}
}

// onAmazonLinux reports whether this machine is Amazon Linux, where send and receive read a real release and rpm.
func onAmazonLinux() bool {
	b, err := os.ReadFile("/etc/os-release")
	return err == nil && regexp.MustCompile(`(?m)^ID="?amzn"?\s*$`).Match(b)
}

// Off a Nitro parent instance, send cannot read the Amazon Linux release or the docker and nitro-cli versions: it says
// so, sends the config anyway, and leaves the three fields out of build_env.
func TestBuildEnv_SendOffAmazonLinuxLeavesNitroFieldsEmpty(t *testing.T) {
	if onAmazonLinux() {
		t.Skip("this machine is Amazon Linux: send reads the real values (e2e/nitro-tools-check.sh checks that on an instance)")
	}
	dir := t.TempDir()
	vendor := build(t, dir, "./cmd/vendor")
	remote, _ := newRemoteRepo(t)
	cfgPath := filepath.Join(dir, "build-config.json")
	if err := os.WriteFile(cfgPath, buildConfigJSON(t, "", nil), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCmd(t, vendor, "send", "--repo", remote, "--config", cfgPath, "--workdir", cloneRemote(t, remote))
	if code != 0 {
		t.Fatalf("send exit %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{"build_env.ami_release left empty", "build_env.docker and nitro_cli left empty"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("send must warn %q, got:\n%s", want, stderr)
		}
	}
	b, err := os.ReadFile(filepath.Join(cloneRemote(t, remote), "build-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ami_release", "docker", "nitro_cli"} {
		if strings.Contains(string(b), `"`+key+`"`) {
			t.Errorf("pushed build-config.json must not carry an empty %s:\n%s", key, b)
		}
	}
}

// receive on aws-nitro compares the Amazon Linux release the Vendor recorded with this machine's. Here the machine is
// not Amazon Linux, so a recorded release is refused naming it; --ignore-env makes it a warning. A config without one
// gets a warning that it was not compared.
func TestBuildEnv_ReceiveComparesAMIRelease(t *testing.T) {
	if onAmazonLinux() {
		t.Skip("this machine is Amazon Linux; the refusal off Amazon Linux cannot be seen here (e2e/nitro-tools-check.sh covers the instance)")
	}
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	remote, seed := newRemoteRepo(t)
	env := map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(), "trimpath": true,
		"ami_release": "2023.12.20261109", "docker": "25.0.18-1.amzn2023.0.3", "nitro_cli": "1.5.0-0.amzn2023"}
	pushConfig(t, remote, buildConfigJSON(t, seed, env))

	stdout, stderr, code := runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code == 0 {
		t.Fatalf("a recorded release on a machine that is not Amazon Linux must be refused\n%s%s", stdout, stderr)
	}
	for _, want := range []string{"ami_release: Vendor built on Amazon Linux 2023.12.20261109", "not Amazon Linux 2023", "--ignore-env"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr must contain %q, got:\n%s", want, stderr)
		}
	}
	if !strings.Contains(stdout, "ami_release=2023.12.20261109 docker=25.0.18-1.amzn2023.0.3 nitro_cli=1.5.0-0.amzn2023 (Vendor)") {
		t.Errorf("the Vendor's build env must be printed with its Nitro fields, got:\n%s", stdout)
	}

	stdout, stderr, code = runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"), "--ignore-env")
	if code != 0 || !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "ami_release") {
		t.Fatalf("--ignore-env must pass with a warning naming ami_release; exit %d\n%s%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "att-auditor build --config") {
		t.Errorf("receive must point at build --config on aws-nitro, got:\n%s", stdout)
	}

	delete(env, "ami_release")
	pushConfig(t, remote, buildConfigJSON(t, seed, env))
	stdout, stderr, code = runCmd(t, auditor, "receive", "--repo", remote, "--into", filepath.Join(t.TempDir(), "src"))
	if code != 0 || !strings.Contains(stderr, "no ami_release") {
		t.Errorf("a config without ami_release: exit 0 and a warning; exit %d\n%s%s", code, stdout, stderr)
	}
}

// build --config takes only an aws-nitro build-config.json, and without --config says the tools are not compared.
// Both happen before the build starts, so they show on any machine.
func TestBuild_ConfigFlag(t *testing.T) {
	dir := t.TempDir()
	auditor := build(t, dir, "./cmd/auditor")
	app := t.TempDir()
	if err := os.WriteFile(filepath.Join(app, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tdx := map[string]any{
		"schema": "attestamp.build-config/1", "service": "svc", "version": "1.2.0", "platform": "intel-tdx",
		"source":    map[string]string{"repo": "https://example.com/svc.git", "commit": "0123456789abcdef"},
		"tee_info":  map[string]string{"instance_name": "g", "project_id": "p", "zone": "z", "image": "i"},
		"build_env": map[string]any{"os": "linux", "arch": "amd64", "go": runtime.Version(), "trimpath": true},
	}
	b, err := json.Marshal(tdx)
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "tdx-config.json")
	if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCmd(t, auditor, "build", "--app-dir", app, "--out-dir", t.TempDir(), "--config", cfgPath)
	if code == 0 || !strings.Contains(stderr, "is for intel-tdx; this build is aws-nitro") {
		t.Errorf("an intel-tdx config must be refused by the Nitro build; exit %d\n%s%s", code, stdout, stderr)
	}
	stdout, stderr, _ = runCmd(t, auditor, "build", "--app-dir", app, "--out-dir", t.TempDir())
	if !strings.Contains(stderr, "no --config, so the docker and nitro-cli versions installed here are not compared") {
		t.Errorf("build without --config must say what it does not compare, got:\n%s%s", stdout, stderr)
	}
}
