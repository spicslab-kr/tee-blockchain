// Behaviour of att-auditor audit settled on 2026-11-16: on a TDX or SEV-SNP format it refuses to register without
// --build-measurements, because it would register whatever the guest reports (a TDX guest booted without an initrd
// was registered that way and then trusted: docs/results/sessions-2026-11-16/tdx-initrdless.md).
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
