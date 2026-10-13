// The responder build-info check (internal/setup/build/responder.go) through the real CLI: actually builds and runs att-vendor.
//   - Off Linux, `build --responder <unrelated file>` ends first with a GOOS error (order: GOOS check, toolchain check,
//     responder check). So on Windows/macOS it only confirms that the responder message never appears.
//   - On Linux, given a responder cross-built for another architecture (an arm64 responder on an amd64 host, or the reverse), it refuses naming both architectures,
//     and a responder built without -trimpath is refused naming -trimpath. The real measurement on an actual Nitro host is
//     e2e/responder-check.sh.
package cmd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	setupbuild "attestamp/internal/setup/build"
)

func TestResponderCheck_E2E(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	bin := build(t, t.TempDir(), "./cmd/vendor")
	app := t.TempDir()
	if err := os.WriteFile(filepath.Join(app, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runBuild := func(responder string) (string, error) {
		out, err := exec.Command(bin, "build", "--app-dir", app, "--out", t.TempDir(), "--responder", responder).CombinedOutput()
		return string(out), err
	}

	if runtime.GOOS != "linux" {
		// the responder check comes after the GOOS check: off Linux, even an unrelated file gets the GOOS error first and no mention of the responder.
		bad := filepath.Join(t.TempDir(), "att-ra-responder")
		if err := os.WriteFile(bad, []byte("not a go binary\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		out, err := runBuild(bad)
		if err == nil {
			t.Fatalf("build on %s should fail:\n%s", runtime.GOOS, out)
		}
		if !strings.Contains(out, "build runs on the Nitro parent instance (linux)") {
			t.Fatalf("expected the GOOS error first, got:\n%s", out)
		}
		if strings.Contains(out, "Go build info") || strings.Contains(out, "att-ra-responder was built") {
			t.Fatalf("responder check ran before the GOOS check:\n%s", out)
		}
		t.Logf("not linux — the responder refusal itself is exercised on the Nitro host by e2e/responder-check.sh")
		return
	}

	// Linux: build a responder for an architecture different from this host's; it must be refused, naming both architectures.
	other := "arm64"
	if runtime.GOARCH == "arm64" {
		other = "amd64"
	}
	wrongArch := filepath.Join(t.TempDir(), "att-ra-responder-"+other)
	if err := setupbuild.BuildResponderForArch(context.Background(), repoRoot(t), setupbuild.ArchTarget{Name: other, GOARCH: other}, wrongArch); err != nil {
		t.Fatal(err)
	}
	out, err := runBuild(wrongArch)
	if err == nil {
		t.Fatalf("build with a %s responder on a %s host should be refused:\n%s", other, runtime.GOARCH, out)
	}
	if !strings.Contains(out, "GOARCH="+other) || !strings.Contains(out, "this instance is "+runtime.GOARCH) {
		t.Fatalf("refusal does not name both architectures:\n%s", out)
	}

	// a responder built without -trimpath: must be refused, naming -trimpath.
	noTrim := filepath.Join(t.TempDir(), "att-ra-responder-notrimpath")
	cmd := exec.Command("go", "build", "-o", noTrim, "./cmd/ra-responder")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build without -trimpath: %v\n%s", err, o)
	}
	out, err = runBuild(noTrim)
	if err == nil {
		t.Fatalf("build with a responder built without -trimpath should be refused:\n%s", out)
	}
	if !strings.Contains(out, "without -trimpath") {
		t.Fatalf("refusal does not name -trimpath:\n%s", out)
	}
}
