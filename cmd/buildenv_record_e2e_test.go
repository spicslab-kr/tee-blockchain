// How registration and records state their build environment, through the real CLI. The same source has different PCRs on x86_64 and arm64, and with a different Go toolchain,
// so a registration and the hand-over record (measurement-record.json) must say which architecture and toolchain they are for.
// The on-chain Entry is unchanged (the measurement hash already differs per architecture, so keys differ on their own); what is checked here is the screen output and
// the record file. register needs anvil (without it only that part is skipped); send runs to the end against a local bare repository, no chain needed.
package cmd_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBuildEnv_E2E_RegisterPrintsArchAndToolchain: register accepts --arch and prints it in the confirmation sentence together with this binary's Go toolchain,
// rejects a wrong spelling, and defaults to this machine's GOARCH when the flag is absent.
func TestBuildEnv_E2E_RegisterPrintsArchAndToolchain(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stop := startAnvil(t)
	defer stop()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
	auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)
	auditorBin := buildBin(t, t.TempDir(), "./cmd/auditor")
	contract := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)

	measurements := func(fill string) string {
		return "PCR0=" + strings.Repeat(fill, 48) + ",PCR1=" + strings.Repeat(fill, 48) + ",PCR2=" + strings.Repeat(fill, 48)
	}
	register := func(extra ...string) ([]byte, error) {
		args := append([]string{"register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
			"--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro"}, extra...)
		return exec.Command(auditorBin, args...).CombinedOutput()
	}
	goVer := runtime.Version() // the test binary and the auditor binary are built by the same (pinned) Go

	// 1. --arch arm64: (arm64, go1.x.y) must show in both the registration sentence and the confirmation block, and the detail and hint lines state the whole build_env.
	out, err := register("--measurements", measurements("a1"), "--arch", "arm64")
	if err != nil {
		t.Fatalf("register --arch arm64: %v\n%s", err, out)
	}
	for _, want := range []string{
		"registering e2e-svc 1.0.0 (arm64, " + goVer,
		"registered e2e-svc 1.0.0 (arm64, " + goVer,
		"build env  linux/arm64 " + goVer + " trimpath=", // the enclave's OS, not this machine's
		`"build_env": {"os":"linux","arch":"arm64","go":"` + goVer + `","trimpath":`,
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("register --arch arm64: output lacks %q:\n%s", want, out)
		}
	}
	if keyLineRe.FindSubmatch(out) == nil {
		t.Fatalf("no registration key in output:\n%s", out)
	}

	// 2. --arch x86_64: the uname spelling is refused, and the message must say which spellings are accepted. Nothing goes to the chain.
	out, err = register("--measurements", measurements("b2"), "--arch", "x86_64")
	if err == nil {
		t.Fatalf("register --arch x86_64 must be rejected:\n%s", out)
	}
	for _, want := range []string{"--arch", "amd64", "arm64", "x86_64"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("rejection must name the accepted spellings, lacks %q:\n%s", want, out)
		}
	}
	if bytes.Contains(out, []byte("registered")) {
		t.Errorf("a rejected --arch must not register anything:\n%s", out)
	}

	// 3. no flag: this machine's GOARCH is the default.
	out, err = register("--measurements", measurements("c3"))
	if err != nil {
		t.Fatalf("register without --arch: %v\n%s", err, out)
	}
	if want := "registered e2e-svc 1.0.0 (" + runtime.GOARCH + ", " + goVer; !bytes.Contains(out, []byte(want)) {
		t.Errorf("default arch must be runtime.GOARCH (%s), output lacks %q:\n%s", runtime.GOARCH, want, out)
	}
}

// pcrB64: the record file's values carry the bytes as base64 (a 48-byte PCR).
func pcrB64(fill byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 48))
}

// writeRecord writes a measurement-record.json in the Go schema (attestamp.measurement-record/1). If buildEnvLine is empty, build_env is left out entirely.
func writeRecord(t *testing.T, dir, name, buildEnvLine string) string {
	t.Helper()
	body := `{
  "schema": "attestamp.measurement-record/1",
  "service": "svc",
  "version": "1.0.0",
  "platform": "aws",
` + buildEnvLine + `  "image": { "path": "svc.eif" },
  "values": [
    { "name": "PCR0", "value": "` + pcrB64(1) + `" },
    { "name": "PCR1", "value": "` + pcrB64(2) + `" },
    { "name": "PCR2", "value": "` + pcrB64(3) + `" }
  ],
  "build_tool": { "name": "nitro-cli", "version": "1.3.0" }
}
`
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// gitBare is a local bare repository for send to push to. Lets send run to the end with no network and no chain.
func gitBare(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", "--initial-branch=main", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	return dir
}

// gitIdentity passes a name and mail through the environment so commits work on any machine (without relying on the user's git config).
func gitIdentity(cmd *exec.Cmd) {
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=att-test", "GIT_AUTHOR_EMAIL=att-test@example.invalid",
		"GIT_COMMITTER_NAME=att-test", "GIT_COMMITTER_EMAIL=att-test@example.invalid")
}

// TestBuildEnv_E2E_SendRequiresBuildEnv: send --record refuses a record without build_env (naming build_env),
// and if present, prints that build environment on one line and sends. The pushed file's build_env.go is exactly what the record says.
func TestBuildEnv_E2E_SendRequiresBuildEnv(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	auditorBin := buildBin(t, t.TempDir(), "./cmd/auditor")
	work := t.TempDir()
	eif := filepath.Join(work, "svc.eif")
	if err := os.WriteFile(eif, []byte("eif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. no build_env: refused. The message must name build_env, and it must fail before any clone or push.
	noEnv := writeRecord(t, work, "no-build-env.json", "")
	cmd := exec.Command(auditorBin, "send", "--repo", gitBare(t), "--record", noEnv, "--image", eif)
	gitIdentity(cmd)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("send with a record lacking build_env must fail:\n%s", out)
	}
	for _, want := range []string{"build_env.os", "build_env.arch", "build_env.go", "amd64|arm64"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("rejection must name build_env and the accepted architectures, lacks %q:\n%s", want, out)
		}
	}
	if bytes.Contains(out, []byte("sent ")) {
		t.Errorf("nothing must be sent without build_env:\n%s", out)
	}

	// 2. build_env present (Go of this toolchain): printed on one line, and actually pushed to the local bare repository.
	goVer := runtime.Version()
	withEnv := writeRecord(t, work, "with-build-env.json",
		`  "build_env": { "os": "linux", "arch": "amd64", "go": "`+goVer+`", "trimpath": true },`+"\n")
	remote := gitBare(t)
	cmd = exec.Command(auditorBin, "send", "--repo", remote, "--record", withEnv, "--image", eif)
	gitIdentity(cmd)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("send with build_env: %v\n%s", err, out)
	}
	if want := "build env: linux/amd64 " + goVer + " trimpath=true"; !bytes.Contains(out, []byte(want)) {
		t.Errorf("send must print the record's build env, lacks %q:\n%s", want, out)
	}
	if bytes.Contains(out, []byte("warning:")) {
		t.Errorf("a record built with this project's toolchain must not draw a toolchain warning:\n%s", out)
	}
	if !bytes.Contains(out, []byte("sent ")) {
		t.Errorf("send must report the push:\n%s", out)
	}
	// The sent file carries build_env too: this is the file the receiver (the Vendor) reads.
	pushed, err := exec.Command("git", "-C", remote, "show", "HEAD:measurement-record.json").CombinedOutput()
	if err != nil {
		t.Fatalf("read pushed record: %v\n%s", err, pushed)
	}
	var rec struct {
		BuildEnv struct {
			OS, Arch, Go string
			Trimpath     bool
		} `json:"build_env"`
	}
	if err := json.Unmarshal(pushed, &rec); err != nil {
		t.Fatalf("pushed measurement-record.json is not JSON: %v\n%s", err, pushed)
	}
	if rec.BuildEnv.OS != "linux" || rec.BuildEnv.Arch != "amd64" || rec.BuildEnv.Go != goVer || !rec.BuildEnv.Trimpath {
		t.Errorf("pushed measurement-record.json build_env drifted: %+v\n%s", rec.BuildEnv, pushed)
	}

	// 3. a record naming a Go of a different toolchain: it is sent, but with a one-line warning (it differs from the Go this project pins).
	otherGo := writeRecord(t, work, "other-go.json",
		`  "build_env": { "os": "linux", "arch": "amd64", "go": "go1.0.0", "trimpath": true },`+"\n")
	cmd = exec.Command(auditorBin, "send", "--repo", gitBare(t), "--record", otherGo, "--image", eif)
	gitIdentity(cmd)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("send with another toolchain's record must still succeed: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("warning:")) || !bytes.Contains(out, []byte("go1.0.0")) {
		t.Errorf("send must warn that the record's toolchain differs from the pinned one:\n%s", out)
	}
}
