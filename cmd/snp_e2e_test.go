// The recorded GCP SEV-SNP evidence driven through the real att-user binary. The current recording booted with an
// initrd, so ra-parse must verify both signatures and the nonce and write the five amd-sev-snp-initrd values. The same
// evidence is not verity evidence, and the older initrdless recording must yield nothing.
package cmd_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"attestamp/internal/tee/parse/snp"
)

const snpNonce = "d529879465187801d4b9ffc1c7d0848a2f028e5df9ab46d3eb7a9d5cdb7f925f"

// snpDoc packs one recording from internal/tee/parse/snp/testdata (prefix "gcp-n2d" or "gcp-n2d-initrdless") into a
// file the way the Vendor sends it.
func snpDoc(t *testing.T, work, prefix string) string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "tee", "parse", "snp", "testdata")
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, prefix+"-"+name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	doc, err := snp.Encode(read("report.bin"), read("auxblob.bin"), read("att.pb"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(work, prefix+".cbor")
	if err := os.WriteFile(p, doc, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCmd_E2E_SNP_ParseInitrdBoot(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	work := t.TempDir()
	docPath := snpDoc(t, work, "gcp-n2d")
	user := build(t, t.TempDir(), "./cmd/user")

	measPath := filepath.Join(work, "initrd.json")
	out, err := exec.Command(user, "ra-parse", "--doc", docPath, "--format", snp.FormatInitrd, "--nonce", snpNonce, "--out-file", measPath).CombinedOutput()
	if err != nil {
		t.Fatalf("ra-parse: %v\n%s", err, out)
	}
	for _, want := range []string{"signature  ok", "nonce      ok", "debug      false", "timestamp  none in the document"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	raw, err := os.ReadFile(measPath)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Format       string
		Measurements map[string]string
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"MEASUREMENT": "f67900851e25a378f04b4d4b5f1ea4410c11deb74e97ddcc7e3363a33f6e1dbb7880d304005387bc98e9cbdceec4ea28",
		"PCR4":        "5b65d2d390d40a5dffb29675a90939615b25aaf164a8b493743242e6f5e84389",
		"KERNEL":      "3cf8c2c34cd30720d3f37c8b5327ae7d10f24073e73a34edb1e4aaea4dd39adb",
		"CMDLINE":     "80dfd7d3a1f7ba936075eb6666f5e72c865fa2c11295ba0f35cfbdb276cc3de0",
		"INITRD":      "4703ec07684196e8616d384ac585be54b6345ca8c6ea5544aab8445e0cd4b0a2",
	}
	if got.Format != snp.FormatInitrd || len(got.Measurements) != len(want) {
		t.Fatalf("measurements file = %s", raw)
	}
	for k, v := range want {
		if got.Measurements[k] != v {
			t.Errorf("%s = %s, want %s", k, got.Measurements[k], v)
		}
	}

	// no root hash on the command line: not verity evidence, and nothing is written
	verityPath := filepath.Join(work, "verity.json")
	out, err = exec.Command(user, "ra-parse", "--doc", docPath, "--format", snp.FormatVerity, "--nonce", snpNonce, "--out-file", verityPath).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "attestamp.roothash=") {
		t.Fatalf("an initrd boot must not pass as verity: %v\n%s", err, out)
	}
	if _, err := os.Stat(verityPath); err == nil {
		t.Error("no measurements file may be written for verity")
	}
}

// The older recording: initrdless, and bound with SHA-512 (the binding until 2026-11-11). It must fail at the binding,
// before any value is read.
func TestCmd_E2E_SNP_OldRecordingIsRefused(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	work := t.TempDir()
	docPath := snpDoc(t, work, "gcp-n2d-initrdless")
	user := build(t, t.TempDir(), "./cmd/user")
	for _, format := range []string{snp.FormatInitrd, snp.FormatVerity} {
		measPath := filepath.Join(work, format+".json")
		out, err := exec.Command(user, "ra-parse", "--doc", docPath, "--format", format, "--nonce", snpNonce, "--out-file", measPath).CombinedOutput()
		if err == nil || !strings.Contains(string(out), "SHA3-512") {
			t.Fatalf("%s: evidence bound with SHA-512 must be refused: %v\n%s", format, err, out)
		}
		if _, err := os.Stat(measPath); err == nil {
			t.Errorf("%s: no measurements file may be written", format)
		}
	}
}
