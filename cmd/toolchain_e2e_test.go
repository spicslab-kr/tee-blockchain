// 툴체인 고정(internal/setup/build/GO_TOOLCHAIN)을 실제 CLI 로 — tb-vendor 를 진짜로 빌드해 실행해 본다.
//   - 고정된 툴체인으로 빌드한 tb-vendor 는 `version` 이 exit 0 이고 고정 버전을 찍는다
//   - `build --app-dir` 은 리눅스가 아닌 곳에서 GOOS 오류가 먼저 난다 (순서: GOOS 검사 → 툴체인 검사)
//   - 다른 패치 버전(GOTOOLCHAIN=go1.26.0)으로 빌드한 tb-vendor 는 `version` 이 exit 1 이고 두 버전을 다 말한다;
//     리눅스에서는 `build --app-dir` 도 같은 메시지로 거부한다. 그 툴체인을 못 받아오는 환경(오프라인)이면 그 절반만 건너뛴다.
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

// wrongToolchain — 고정 버전과 다른 패치 버전. 1.26.1 이 고정이므로 1.26.0 은 항상 다르다.
const wrongToolchain = "go1.26.0"

// buildWith — build() 와 같되 GOTOOLCHAIN 을 강제한다. 그 툴체인을 못 받아오면 ok=false.
func buildWith(t *testing.T, dir, pkg, gotoolchain string) (bin string, ok bool) {
	t.Helper()
	probe := exec.Command("go", "version")
	probe.Env = append(os.Environ(), "GOTOOLCHAIN="+gotoolchain)
	if out, err := probe.CombinedOutput(); err != nil || !strings.Contains(string(out), gotoolchain) {
		t.Logf("GOTOOLCHAIN=%s 을 쓸 수 없음 (오프라인?): %v\n%s", gotoolchain, err, out)
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

	// build --app-dir: 리눅스가 아니면 GOOS 오류가 먼저 — 툴체인 메시지가 아니라. (순서 증명)
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
