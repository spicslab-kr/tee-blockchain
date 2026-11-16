// Two behaviours of att-auditor audit settled on 2026-11-16:
//   - on a TDX or SEV-SNP format it refuses to register without --build-measurements, because it would register
//     whatever the guest reports (a TDX guest booted without an initrd was registered that way and then trusted:
//     docs/results/sessions-2026-11-16/tdx-initrdless.md);
//   - with --vendor-tee, a Vendor TEE that runs something else ends with exit 12 after the Auditor's own value is
//     registered, and the message says the registration stands and not to run audit again.
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

func TestRun_E2E_AuditRefusesCVMWithoutBuild(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	userBin := buildBin(t, bins, "./cmd/user")

	tdxDir := filepath.Join(repoRoot(t), "internal", "tee", "parse", "tdx", "testdata")
	tdxNonce := sha512.Sum512([]byte("attestamp-tdx-opt1-v1"))
	snpNonce := sha256.Sum256([]byte("attestamp-snp-opt1p-v1"))
	cases := []struct {
		name, format, doc, nonce, guide string
	}{
		{"tdx", "intel-tdx", filepath.Join(tdxDir, "gcp-c3-opt1-v1-quote.bin"), hex.EncodeToString(tdxNonce[:]), "docs/build/tdx.md"},
		{"snp", snp.FormatInitrd, snpDoc(t, t.TempDir(), "gcp-n2d-opt1p-v1"), hex.EncodeToString(snpNonce[:]), "docs/build/sev-snp.md"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rpc, stopAnvil := startAnvil(t)
			defer stopAnvil()
			keys := t.TempDir()
			deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
			auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)
			registryAddr := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)
			addr, stop := startServeAs(t, vendorBin, c.doc, c.format)
			defer stop()

			// --tee-addr alone: the attested values would be registered as they are. Refused before any request.
			out, err := exec.Command(auditorBin, "audit", "--tee-addr", addr, "--format", c.format,
				"--rpc", rpc, "--registry", registryAddr, "--signing-key-file", auditorFile,
				"--service", "no-build", "--version", "1", "--nonce", c.nonce).CombinedOutput()
			if err == nil {
				t.Fatalf("audit without --build-measurements on %s must be refused:\n%s", c.format, out)
			}
			for _, want := range []string{"without --build-measurements", "whatever the guest reports", c.guide} {
				if !strings.Contains(string(out), want) {
					t.Errorf("the refusal must say %q:\n%s", want, out)
				}
			}

			// Nothing was registered: the guest's values are unknown to the registry.
			parsed := filepath.Join(t.TempDir(), "parsed.json")
			if pOut, err := exec.Command(userBin, "ra-parse", "--doc", c.doc, "--format", c.format, "--nonce", c.nonce,
				"--out-file", parsed).CombinedOutput(); err != nil {
				t.Fatalf("ra-parse: %v\n%s", err, pOut)
			}
			lOut, err := exec.Command(userBin, "lookup", "--rpc", rpc, "--registry", registryAddr, "--measurements", parsed,
				"--platform", c.format).CombinedOutput()
			if err != nil || !strings.Contains(string(lOut), "nothing registered") {
				t.Fatalf("nothing may be registered after the refusal: %v\n%s", err, lOut)
			}
		})
	}
}

func TestRun_E2E_AuditVendorDiffersExits12(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")

	tdxDir := filepath.Join(repoRoot(t), "internal", "tee", "parse", "tdx", "testdata")
	nonceOf := func(cell string) string {
		s := sha512.Sum512([]byte("attestamp-tdx-" + cell))
		return hex.EncodeToString(s[:])
	}
	v1, v2 := filepath.Join(tdxDir, "gcp-c3-opt1-v1-quote.bin"), filepath.Join(tdxDir, "gcp-c3-opt1-v2-quote.bin")

	rpc, stopAnvil := startAnvil(t)
	defer stopAnvil()
	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
	auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)
	registryAddr := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)
	work := t.TempDir()

	// The Auditor built v1 and its own guest runs v1; the Vendor's guest runs v2, one byte different.
	built := filepath.Join(work, "built-v1.json")
	if out, err := exec.Command(auditorBin, "ra-parse", "--doc", v1, "--format", "intel-tdx", "--nonce", nonceOf("opt1-v1"),
		"--out-file", built).CombinedOutput(); err != nil {
		t.Fatalf("ra-parse v1: %v\n%s", err, out)
	}
	mine, stopMine := startServeAs(t, vendorBin, v1, "intel-tdx")
	defer stopMine()
	theirs, stopTheirs := startServeAs(t, vendorBin, v2, "intel-tdx")
	defer stopTheirs()

	args := []string{"audit", "--tee-addr", mine, "--format", "intel-tdx", "--build-measurements", built,
		"--vendor-tee", theirs, "--rpc", rpc, "--registry", registryAddr, "--signing-key-file", auditorFile,
		"--service", "vendor-differs", "--version", "1", "--nonce", nonceOf("opt1-v1"), "--vendor-nonce", nonceOf("opt1-v2")}
	out, err := exec.Command(auditorBin, args...).CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 12 {
		t.Fatalf("a Vendor that runs something else must end audit with exit 12: %v\n%s", err, out)
	}
	for _, want := range []string{"the Vendor's guest runs something else", "PASS: audited and registered",
		"registration above SUCCEEDED", "Only the comparison", "Do not run audit again", "exit 5"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("audit's output must say %q:\n%s", want, out)
		}
	}

	// The registration stands: running audit again meets "already registered" (exit 5), as the message warns.
	out, err = exec.Command(auditorBin, args...).CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 5 {
		t.Fatalf("a second audit after exit 12 must meet 'already registered' (exit 5): %v\n%s", err, out)
	}
}
