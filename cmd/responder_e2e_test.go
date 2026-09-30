// 응답기 빌드 정보 검사(internal/setup/build/responder.go)를 실제 CLI 로 — tb-vendor 를 진짜로 빌드해 실행해 본다.
//   - 리눅스가 아닌 곳에서는 `build --responder <엉뚱한 파일>` 이 GOOS 오류로 먼저 끝난다 (순서: GOOS 검사 → 툴체인 검사
//     → 응답기 검사). 그래서 Windows/macOS 에서는 응답기 메시지가 절대 나오지 않는다는 것만 확인한다.
//   - 리눅스에서는 다른 아키텍처(amd64 호스트에 arm64 응답기, 또는 그 반대)로 크로스빌드한 응답기를 주면 두 아키텍처를
//     다 말하며 거부하고, -trimpath 없이 빌드한 응답기는 -trimpath 를 말하며 거부한다. 실제 Nitro 호스트에서의 실측은
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

	setupbuild "tb/internal/setup/build"
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
		// 응답기 검사는 GOOS 검사 뒤에 온다: 리눅스가 아니면 엉뚱한 파일을 줘도 GOOS 오류가 먼저 나고 응답기 얘기는 없다.
		bad := filepath.Join(t.TempDir(), "tb-ra-responder")
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
		if strings.Contains(out, "Go build info") || strings.Contains(out, "tb-ra-responder was built") {
			t.Fatalf("responder check ran before the GOOS check:\n%s", out)
		}
		t.Logf("not linux — the responder refusal itself is exercised on the Nitro host by e2e/responder-check.sh")
		return
	}

	// 리눅스: 이 호스트와 다른 아키텍처의 응답기를 만들어 준다 — 두 아키텍처를 다 말하며 거부해야 한다.
	other := "arm64"
	if runtime.GOARCH == "arm64" {
		other = "amd64"
	}
	wrongArch := filepath.Join(t.TempDir(), "tb-ra-responder-"+other)
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

	// -trimpath 없이 빌드한 응답기 — -trimpath 를 말하며 거부해야 한다.
	noTrim := filepath.Join(t.TempDir(), "tb-ra-responder-notrimpath")
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
