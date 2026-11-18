// ra-serve with the fixture extractor serves a recorded document under the format --format names, in both the
// Vendor's and the Auditor's binary (the two role copies of ra-serve are the same).
package cmd_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestServe_E2E_FixtureFormatInBothRoles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	bins := t.TempDir()
	userBin := buildBin(t, bins, "./cmd/user")
	quote := filepath.Join(repoRoot(t), "internal", "tee", "parse", "tdx", "testdata", "gcp-c3-opt1-v1-quote.bin")

	for _, role := range []string{"vendor", "auditor"} {
		t.Run(role, func(t *testing.T) {
			bin := buildBin(t, bins, "./cmd/"+role)

			// served as intel-tdx: a request that expects intel-tdx gets it
			addr, stop := startServeAs(t, bin, quote, "intel-tdx")
			out, err := exec.Command(userBin, "ra-request", "--tee-addr", addr, "--format", "intel-tdx",
				"--out-file", filepath.Join(t.TempDir(), "doc.bin")).CombinedOutput()
			stop()
			if err != nil {
				t.Fatalf("%s ra-serve --format intel-tdx: the request for intel-tdx failed: %v\n%s", role, err, out)
			}

			// without --format the fixture is served as aws-nitro, and the same request is refused
			addr, stop = startServeAs(t, bin, quote, "")
			out, err = exec.Command(userBin, "ra-request", "--tee-addr", addr, "--format", "intel-tdx",
				"--out-file", filepath.Join(t.TempDir(), "doc.bin")).CombinedOutput()
			stop()
			if err == nil || !strings.Contains(string(out), "format") {
				t.Fatalf("%s ra-serve without --format must serve aws-nitro, which an intel-tdx request refuses: %v\n%s", role, err, out)
			}
		})
	}
}
