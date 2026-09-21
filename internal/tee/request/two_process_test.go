// 두 프로세스 — vendor 바이너리가 TEE 쪽 ra-serve 를 띄우고, user 바이너리가 ra-request 로 문서를 받는다.
package request_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func build(t *testing.T, dir, pkg string) string {
	t.Helper()
	bin := filepath.Join(dir, filepath.Base(pkg))
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			t.Fatal("go.mod not found")
		}
	}
}

func startServe(t *testing.T, bin, fixture string) (addr string, stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "ra-serve", "--listen", "tcp:127.0.0.1:0", "--extractor", "fixture="+fixture)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		cancel()
		t.Fatalf("serve did not report an address: %v", err)
	}
	i := strings.LastIndex(line, "on ")
	if i < 0 {
		cancel()
		t.Fatalf("unexpected serve output: %q", line)
	}
	return strings.TrimSpace(line[i+3:]), func() { cancel(); _ = cmd.Wait() }
}

func TestRA_TwoProcesses_UserRequestsVendorTEE(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	fixture, _ := filepath.Abs(filepath.Join("testdata", "nitro-attestation.cbor"))
	want, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	bins := t.TempDir()
	vendor := build(t, bins, "./cmd/vendor")
	user := build(t, bins, "./cmd/user")

	addr, stop := startServe(t, vendor, fixture)
	defer stop()

	out := filepath.Join(t.TempDir(), "doc.cbor")
	req := exec.Command(user, "ra-request", "--tee", addr, "--out", out, "--timeout", "10s")
	res, err := req.CombinedOutput()
	if err != nil {
		t.Fatalf("ra-request: %v\n%s", err, res)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("document changed in transit: %x vs %x", sha256.Sum256(got), sha256.Sum256(want))
	}
	nonce, err := os.ReadFile(out + ".nonce")
	if err != nil || len(strings.TrimSpace(string(nonce))) != 64 {
		t.Fatalf("nonce file: %v %q", err, nonce)
	}
	if !strings.Contains(string(res), "aws-nitro document") {
		t.Errorf("output: %s", res)
	}

	// 다른 형식을 기대하면 문서를 받아도 거부한다 (exit 4)
	req = exec.Command(user, "ra-request", "--tee", addr, "--out", out+".2", "--format", "gcp-cs-jwt", "--timeout", "10s")
	res, err = req.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 4 {
		t.Fatalf("want exit 4 (format mismatch), got %v\n%s", err, res)
	}
	if _, err := os.Stat(out + ".2"); err == nil {
		t.Fatal("a rejected document must not be written")
	}
}

func TestRA_TwoProcesses_ServerDownExitCode(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	user := build(t, t.TempDir(), "./cmd/user")
	req := exec.Command(user, "ra-request", "--tee", "127.0.0.1:1", "--timeout", "2s")
	out, err := req.CombinedOutput()
	if err == nil {
		t.Fatalf("must fail when no TEE listens\n%s", out)
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 {
		t.Fatalf("want exit 3 (network), got %v\n%s", err, out)
	}
	_ = time.Second
}
