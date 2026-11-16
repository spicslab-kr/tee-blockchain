// build and launch --platform intel-tdx and amd-sev-snp-*, through the real att-vendor and att-auditor binaries. On Linux both roles
// build the same initrd from the same files and base initrd (Option 1 needs no external tool). Elsewhere the build
// refuses (file modes are part of the archive), which still shows the flag reaches the TDX build. The launch itself
// needs a TDX guest (docs/status/cvm-port.md, "Phase 3: what waits for a VM").
package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCmd_E2E_TDXBuild_VendorAndAuditorAgree(t *testing.T) {
	bins := t.TempDir()
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	auditorBin := buildBin(t, bins, "./cmd/auditor")

	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "app"), []byte("attestamp app v1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "initrd.img-base")
	if err := os.WriteFile(base, bytes.Repeat([]byte{0x28, 0xb5, 0x2f}, 333), 0o644); err != nil {
		t.Fatal(err)
	}

	var initrds [][]byte
	for _, bin := range []string{vendorBin, auditorBin} {
		out := filepath.Join(t.TempDir(), "out")
		cmd := exec.Command(bin, "build", "--platform", "intel-tdx", "--app-dir", app, "--base-initrd", base, "--option", "1", "--out-dir", out, "--verify")
		got, err := cmd.CombinedOutput()
		if runtime.GOOS != "linux" {
			if err == nil || !strings.Contains(string(got), "runs on Linux") {
				t.Fatalf("%s: want the Linux refusal, got %v\n%s", filepath.Base(bin), err, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v\n%s", filepath.Base(bin), err, got)
		}
		if !strings.Contains(string(got), "reproducible on this host") {
			t.Fatalf("%s: --verify gave no verdict\n%s", filepath.Base(bin), got)
		}
		b, err := os.ReadFile(filepath.Join(out, "initrd-app.img"))
		if err != nil {
			t.Fatal(err)
		}
		initrds = append(initrds, b)
	}
	if len(initrds) == 2 && !bytes.Equal(initrds[0], initrds[1]) {
		t.Fatal("the Vendor's and the Auditor's builds of the same files differ")
	}
}

// The same for SEV-SNP Option 1: on Linux both roles build the same initrd, and its archive is the one the recorded
// guest booted (internal/setup/build/snp checks the archive hash); elsewhere the build refuses.
func TestCmd_E2E_SNPBuild_VendorAndAuditorAgree(t *testing.T) {
	bins := t.TempDir()
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "app"), []byte("attestamp app v1\n"), 0o644)
	base := filepath.Join(dir, "initrd.img-base")
	os.WriteFile(base, bytes.Repeat([]byte{0x28, 0xb5, 0x2f}, 333), 0o644)
	var initrds [][]byte
	for _, pkg := range []string{"./cmd/vendor", "./cmd/auditor"} {
		bin := buildBin(t, bins, pkg)
		out := filepath.Join(t.TempDir(), "out")
		got, err := exec.Command(bin, "build", "--platform", "amd-sev-snp-initrd", "--app-dir", app, "--base-initrd", base, "--out-dir", out, "--verify").CombinedOutput()
		if runtime.GOOS != "linux" {
			if err == nil || !strings.Contains(string(got), "runs on Linux") {
				t.Fatalf("%s: want the Linux refusal, got %v\n%s", pkg, err, got)
			}
			continue
		}
		if err != nil || !strings.Contains(string(got), "reproducible on this host") {
			t.Fatalf("%s: %v\n%s", pkg, err, got)
		}
		b, _ := os.ReadFile(filepath.Join(out, "initrd-app.img"))
		initrds = append(initrds, b)
	}
	if len(initrds) == 2 && !bytes.Equal(initrds[0], initrds[1]) {
		t.Fatal("the Vendor's and the Auditor's builds of the same files differ")
	}
}

func TestCmd_E2E_PlatformFlagRouting(t *testing.T) {
	bins := t.TempDir()
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"build", "--platform", "gcp-cs-jwt", "--app-dir", "x"}, "build supports aws-nitro, intel-tdx, amd-sev-snp-initrd and amd-sev-snp-verity"},
		{[]string{"build", "--platform", "amd-sev-snp-verity", "--app-dir", "x"}, "--base-initrd are required"},
		{[]string{"launch", "--platform", "amd-sev-snp-initrd", "--build-value", filepath.Join(bins, "none.json")}, "none.json"},
		{[]string{"launch", "--platform", "amd-sev-snp-verity", "--check", "--build-value", filepath.Join(bins, "none.json")}, "none.json"},
		{[]string{"build", "--platform=intel-tdx", "--app-dir", "x"}, "--base-initrd are required"},
		{[]string{"launch", "--platform", "intel-tdx"}, "--initrd is required"},
		{[]string{"launch", "--platform", "intel-tdx", "--check", "--build-value", filepath.Join(bins, "none.json")}, "none.json"},
		{[]string{"build", "--platform", "aws-nitro"}, "--app-dir is required"}, // the Nitro path, as before
	} {
		got, err := exec.Command(vendorBin, c.args...).CombinedOutput()
		if err == nil || !strings.Contains(string(got), c.want) {
			t.Errorf("%v: want an error with %q, got %v\n%s", c.args, c.want, err, got)
		}
	}
}
