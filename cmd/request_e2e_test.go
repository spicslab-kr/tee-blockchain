// 두 프로세스 — vendor 바이너리가 TEE 쪽 ra-serve 를 띄우고, user 바이너리가 ra-request 로 문서를 받는다.
// build · repoRoot · startServe 는 e2e_helpers_test.go 에 있다 (parse_e2e_test.go 와 공유).
package cmd_test

import (
	"bytes"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRA_TwoProcesses_UserRequestsVendorTEE(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	fixture, _ := filepath.Abs(filepath.Join(repoRoot(t), "internal", "tee", "request", "testdata", "nitro-attestation.cbor"))
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
