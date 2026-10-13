// The toolchain pin (internal/setup/build/GO_TOOLCHAIN) through the real CLI: actually builds and runs tb-vendor.
//   - tb-vendor built with the pinned toolchain: `version` exits 0 and prints the pinned version
//   - `build --app-dir` hits the GOOS error first off Linux (order: GOOS check, toolchain check)
//   - tb-vendor built with a newer patch version (GOTOOLCHAIN=go1.26.2): `version` exits 1 and names both versions;
//     on Linux, `build --app-dir` refuses with the same message. If that toolchain cannot be fetched (offline), only that half is skipped.
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	toolchain "tb/internal/setup/build"
)

// wrongToolchain is a patch version different from the pinned one. go.mod requires go >= 1.26.1 and 1.26.1 is the pin, so the newer 1.26.2 always differs.
const wrongToolchain = "go1.26.2"

// buildWith is like build() but forces GOTOOLCHAIN. ok=false if that toolchain cannot be fetched.
func buildWith(t *testing.T, dir, pkg, gotoolchain string) (bin string, ok bool) {
	t.Helper()
	probe := exec.Command("go", "version")
	probe.Env = append(os.Environ(), "GOTOOLCHAIN="+gotoolchain)
	if out, err := probe.CombinedOutput(); err != nil || !strings.Contains(string(out), gotoolchain) {
		t.Logf("GOTOOLCHAIN=%s unavailable (offline?): %v\n%s", gotoolchain, err, out)
		return "", false
	}
	bin = filepath.Join(dir, filepath.Base(pkg)+"-"+gotoolchain)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN="+gotoolchain)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s with %s: %v\n%s", pkg, gotoolchain, err, out)
	}
	return bin, true
}

func TestToolchain_E2E_PinnedBinaryPasses(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	bin := build(t, t.TempDir(), "./cmd/vendor")

	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version: %v\n%s", err, out)
	}
	want := toolchain.RequiredToolchain()
	if !strings.Contains(string(out), "toolchain: "+want+" (pinned") || !strings.Contains(string(out), "this binary: "+want) {
		t.Fatalf("version output does not report the pin %s on both sides:\n%s", want, out)
	}

	// build --app-dir: off Linux the GOOS error comes first, not the toolchain message (proves the order).
	if runtime.GOOS != "linux" {
		app := t.TempDir()
		if err := os.WriteFile(filepath.Join(app, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(bin, "build", "--app-dir", app, "--out", t.TempDir()).CombinedOutput()
		if err == nil {
			t.Fatalf("build on %s should fail:\n%s", runtime.GOOS, out)
		}
		if !strings.Contains(string(out), "build runs on the Nitro parent instance (linux)") {
			t.Fatalf("expected the GOOS error first, got:\n%s", out)
		}
		if strings.Contains(string(out), "pins "+want) {
			t.Fatalf("toolchain check ran before the GOOS check:\n%s", out)
		}
	}
}

func TestToolchain_E2E_WrongToolchainRefused(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	want := toolchain.RequiredToolchain()
	if want == wrongToolchain {
		t.Fatalf("test constant wrongToolchain=%s equals the pin — change it", wrongToolchain)
	}
	bin, ok := buildWith(t, t.TempDir(), "./cmd/vendor", wrongToolchain)
	if !ok {
		t.Skipf("cannot obtain %s here (offline?) — the refusal path is covered by internal/setup/build's unit test", wrongToolchain)
	}

	out, err := exec.Command(bin, "version").CombinedOutput()
	if err == nil {
		t.Fatalf("version of a %s-built binary should exit non-zero:\n%s", wrongToolchain, out)
	}
	for _, needle := range []string{"built with " + wrongToolchain, "pins " + want, "GO_TOOLCHAIN"} {
		if !strings.Contains(string(out), needle) {
			t.Errorf("version output lacks %q:\n%s", needle, out)
		}
	}

	if runtime.GOOS != "linux" {
		t.Logf("not linux — build's own refusal is exercised on the Nitro host by e2e/toolchain-pin-check.sh")
		return
	}
	app := t.TempDir()
	if err := os.WriteFile(filepath.Join(app, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = exec.Command(bin, "build", "--app-dir", app, "--out", t.TempDir()).CombinedOutput()
	if err == nil {
		t.Fatalf("build with a %s-built binary should be refused:\n%s", wrongToolchain, out)
	}
	if !strings.Contains(string(out), "pins "+want) || !strings.Contains(string(out), "built with "+wrongToolchain) {
		t.Fatalf("build did not refuse with the toolchain message:\n%s", out)
	}
}
