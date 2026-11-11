// Test-matrix row T5 as code: an application swapped after registration is not trusted. For each platform and option,
// two recordings from the same guest differ only in one byte of the application (docs/plan/app-fixture-recording.md).
// The Auditor audits the guest serving v1 and registers it on a local anvil chain; the User is told v1 is trusted. The
// guest is then replaced by one serving v2, and verify must exit 1 because nobody registered those measurements.
package cmd_test

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"attestamp/internal/tee/parse/snp"
)

func TestRun_E2E_T5_ApplicationSwapIsRefused(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	userBin := buildBin(t, bins, "./cmd/user")

	tdxDir := filepath.Join(repoRoot(t), "internal", "tee", "parse", "tdx", "testdata")
	tdxNonce := func(cell string) string {
		s := sha512.Sum512([]byte("attestamp-tdx-" + cell))
		return hex.EncodeToString(s[:])
	}
	snpNonceOf := func(cell string) string {
		s := sha256.Sum256([]byte("attestamp-snp-" + cell))
		return hex.EncodeToString(s[:])
	}
	docs := t.TempDir()

	cases := []struct {
		name, format string
		v1, v1Nonce  string
		v2, v2Nonce  string
	}{
		{"tdx-option1", "intel-tdx",
			filepath.Join(tdxDir, "gcp-c3-opt1-v1-quote.bin"), tdxNonce("opt1-v1"),
			filepath.Join(tdxDir, "gcp-c3-opt1-v2-quote.bin"), tdxNonce("opt1-v2")},
		{"tdx-option2", "intel-tdx",
			filepath.Join(tdxDir, "gcp-c3-opt2-v1-quote.bin"), tdxNonce("opt2-v1"),
			filepath.Join(tdxDir, "gcp-c3-opt2-v2-quote.bin"), tdxNonce("opt2-v2")},
		// the padded Option 1 pair: the one whose application was really unpacked (testdata/README.md)
		{"snp-option1", snp.FormatInitrd,
			snpDoc(t, docs, "gcp-n2d-opt1p-v1"), snpNonceOf("opt1p-v1"),
			snpDoc(t, docs, "gcp-n2d-opt1p-v2"), snpNonceOf("opt1p-v2")},
		{"snp-option2", snp.FormatVerity,
			snpDoc(t, docs, "gcp-n2d-opt2-v1"), snpNonceOf("opt2-v1"),
			snpDoc(t, docs, "gcp-n2d-opt2-v2"), snpNonceOf("opt2-v2")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rpc, stopAnvil := startAnvil(t)
			defer stopAnvil()
			keys := t.TempDir()
			deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
			auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)
			registryAddr := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)
			work := t.TempDir()

			// The Auditor's own build of v1 measures what the v1 guest attests.
			built := filepath.Join(work, "built-v1.json")
			if out, err := exec.Command(auditorBin, "ra-parse", "--doc", c.v1, "--format", c.format, "--nonce", c.v1Nonce,
				"--out-file", built).CombinedOutput(); err != nil {
				t.Fatalf("ra-parse v1: %v\n%s", err, out)
			}

			// v1 is running: audit registers it, and the User trusts it.
			v1Addr, stopV1 := startServeAs(t, vendorBin, c.v1, c.format)
			aOut, err := exec.Command(auditorBin, "audit", "--tee-addr", v1Addr, "--format", c.format, "--build-measurements", built,
				"--rpc", rpc, "--registry", registryAddr, "--signing-key-file", auditorFile, "--service", "t5-"+c.name, "--version", "1.0.0",
				"--nonce", c.v1Nonce, "--out-dir", filepath.Join(work, "audit")).CombinedOutput()
			if err != nil {
				stopV1()
				t.Fatalf("audit v1: %v\n%s", err, aOut)
			}
			uOut, err := exec.Command(userBin, "verify", "--tee-addr", v1Addr, "--format", c.format, "--rpc", rpc, "--registry", registryAddr,
				"--nonce", c.v1Nonce, "--out-dir", t.TempDir()).CombinedOutput()
			stopV1()
			if err != nil || !strings.Contains(string(uOut), "trusted:") {
				t.Fatalf("v1 must be trusted after its registration: %v\n%s", err, uOut)
			}

			// The application is swapped: the guest now runs v2, one byte different. Nothing registered it.
			v2Addr, stopV2 := startServeAs(t, vendorBin, c.v2, c.format)
			defer stopV2()
			uOut, err = exec.Command(userBin, "verify", "--tee-addr", v2Addr, "--format", c.format, "--rpc", rpc, "--registry", registryAddr,
				"--nonce", c.v2Nonce, "--out-dir", t.TempDir()).CombinedOutput()
			if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 || !strings.Contains(string(uOut), "nobody has registered") {
				t.Fatalf("v2 must be refused with exit 1, 'nobody has registered': %v\n%s", err, uOut)
			}
		})
	}
}
