// The recorded GCP SEV-SNP evidence driven through the real att-user binary. The fixture's guest booted without an
// initrd, so ra-parse must verify both signatures and the nonce and then refuse to register anything.
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"attestamp/internal/tee/parse/snp"
)

func TestCmd_E2E_SNP_ParseVerifiesThenRefusesInitrdlessBoot(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	dir := filepath.Join(repoRoot(t), "internal", "tee", "parse", "snp", "testdata")
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	doc, err := snp.Encode(read("gcp-n2d-report.bin"), read("gcp-n2d-auxblob.bin"), read("gcp-n2d-att.pb"))
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	docPath := filepath.Join(work, "evidence.cbor")
	if err := os.WriteFile(docPath, doc, 0o644); err != nil {
		t.Fatal(err)
	}

	user := build(t, t.TempDir(), "./cmd/user")
	for _, format := range []string{snp.FormatInitrd, snp.FormatVerity} {
		out, err := exec.Command(user, "ra-parse", "--doc", docPath, "--format", format,
			"--nonce", "d529879465187801d4b9ffc1c7d0848a2f028e5df9ab46d3eb7a9d5cdb7f925f",
			"--out-file", filepath.Join(work, format+".json")).CombinedOutput()
		if err == nil {
			t.Fatalf("%s: an initrdless boot must not yield measurements:\n%s", format, out)
		}
		for _, want := range []string{"signature  ok", "nonce      ok", "debug      false", "no initrd"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s: output lacks %q:\n%s", format, want, out)
			}
		}
		if _, err := os.Stat(filepath.Join(work, format+".json")); err == nil {
			t.Errorf("%s: no measurements file may be written", format)
		}
	}
}
