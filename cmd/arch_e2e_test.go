// 등록·기록의 아키텍처 표기를 실제 CLI 로 — 같은 소스도 x86_64 와 arm64 에서 PCR 이 다르니, 등록(register)과
// 전달 기록(measurement-record.json)은 어느 아키텍처 것인지 말해야 한다. 체인 Entry 는 그대로다(측정값 해시가
// 이미 아키텍처마다 다르니 키는 저절로 갈린다) — 여기서 보는 건 화면 출력과 기록 파일의 검사뿐이다.
// register 는 anvil 이 필요하고(없으면 그 부분만 건너뛴다), send 는 로컬 bare 저장소로 체인 없이 끝까지 돈다.
package cmd_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestArch_E2E_RegisterPrintsArch — register 가 --arch 를 받아 확인 문장에 찍고, 잘못된 철자는 거절하고, 없으면 이 기계의 GOARCH.
func TestArch_E2E_RegisterPrintsArch(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stop := startAnvil(t)
	defer stop()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
	auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)
	auditorBin := buildBin(t, t.TempDir(), "./cmd/auditor")
	contract := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)

	measurements := func(fill string) string {
		return "PCR0=" + strings.Repeat(fill, 48) + ",PCR1=" + strings.Repeat(fill, 48) + ",PCR2=" + strings.Repeat(fill, 48)
	}
	register := func(extra ...string) ([]byte, error) {
		args := append([]string{"register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
			"--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro"}, extra...)
		return exec.Command(auditorBin, args...).CombinedOutput()
	}

	// 1. --arch arm64 — 등록 문장과 확인 블록 둘 다에 (arm64) 가 보여야 한다.
	out, err := register("--measurements", measurements("a1"), "--arch", "arm64")
	if err != nil {
		t.Fatalf("register --arch arm64: %v\n%s", err, out)
	}
	for _, want := range []string{"registering e2e-svc 1.0.0 (arm64)", "registered e2e-svc 1.0.0 (arm64)", "arch    arm64", `"arch": "arm64"`} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("register --arch arm64: output lacks %q:\n%s", want, out)
		}
	}
	if keyLineRe.FindSubmatch(out) == nil {
		t.Fatalf("no registration key in output:\n%s", out)
	}

	// 2. --arch x86_64 — uname 철자는 거절. 어떤 철자를 받는지 말해 줘야 한다. 체인에는 아무것도 가지 않는다.
	out, err = register("--measurements", measurements("b2"), "--arch", "x86_64")
	if err == nil {
		t.Fatalf("register --arch x86_64 must be rejected:\n%s", out)
	}
	for _, want := range []string{"--arch", "amd64", "arm64", "x86_64"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("rejection must name the accepted spellings, lacks %q:\n%s", want, out)
		}
	}
	if bytes.Contains(out, []byte("registered")) {
		t.Errorf("a rejected --arch must not register anything:\n%s", out)
	}

	// 3. 플래그 없음 — 이 기계의 GOARCH 가 기본값.
	out, err = register("--measurements", measurements("c3"))
	if err != nil {
		t.Fatalf("register without --arch: %v\n%s", err, out)
	}
	if want := "registered e2e-svc 1.0.0 (" + runtime.GOARCH + ")"; !bytes.Contains(out, []byte(want)) {
		t.Errorf("default arch must be runtime.GOARCH (%s), output lacks %q:\n%s", runtime.GOARCH, want, out)
	}
}

// pcrB64 — 기록 파일의 values 는 바이트를 base64 로 싣는다 (48 바이트 PCR).
func pcrB64(fill byte) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 48)) }

// writeRecord — Go 스키마(tb.measurement-record/1)의 measurement-record.json. archLine 이 비면 arch 를 아예 뺀다.
func writeRecord(t *testing.T, dir, name, archLine string) string {
	t.Helper()
	body := `{
  "schema": "tb.measurement-record/1",
  "service": "svc",
  "version": "1.0.0",
  "platform": "aws",
` + archLine + `  "image": { "path": "svc.eif" },
  "values": [
    { "name": "PCR0", "value": "` + pcrB64(1) + `" },
    { "name": "PCR1", "value": "` + pcrB64(2) + `" },
    { "name": "PCR2", "value": "` + pcrB64(3) + `" }
  ],
  "build_tool": { "name": "nitro-cli", "version": "1.3.0" }
}
`
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// gitBare — send 가 push 할 로컬 bare 저장소. 네트워크도 체인도 없이 send 를 끝까지 돌린다.
func gitBare(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", "--initial-branch=main", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	return dir
}

// gitIdentity — 어느 기계에서든 commit 이 되게 이름·메일을 환경으로 준다 (사용자 git 설정에 기대지 않는다).
func gitIdentity(cmd *exec.Cmd) {
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=tb-test", "GIT_AUTHOR_EMAIL=tb-test@example.invalid",
		"GIT_COMMITTER_NAME=tb-test", "GIT_COMMITTER_EMAIL=tb-test@example.invalid")
}

// TestArch_E2E_SendRequiresArch — send --record 는 arch 가 빠진 기록을 거절하고(arch 를 지목해서), 있으면 그 arch 를 찍고 보낸다.
func TestArch_E2E_SendRequiresArch(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	auditorBin := buildBin(t, t.TempDir(), "./cmd/auditor")
	work := t.TempDir()
	eif := filepath.Join(work, "svc.eif")
	if err := os.WriteFile(eif, []byte("eif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. arch 없음 — 거절. 메시지가 arch 를 지목해야 하고, clone 도 push 도 하기 전에 걸려야 한다.
	noArch := writeRecord(t, work, "no-arch.json", "")
	cmd := exec.Command(auditorBin, "send", "--repo", gitBare(t), "--record", noArch, "--image", eif)
	gitIdentity(cmd)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("send with a record lacking arch must fail:\n%s", out)
	}
	if !bytes.Contains(out, []byte("arch")) || !bytes.Contains(out, []byte("amd64|arm64")) {
		t.Errorf("rejection must name arch and the accepted values:\n%s", out)
	}
	if bytes.Contains(out, []byte("sent ")) {
		t.Errorf("nothing must be sent without arch:\n%s", out)
	}

	// 2. arch: amd64 — 찍고, 로컬 bare 저장소에 실제로 push 된다.
	withArch := writeRecord(t, work, "with-arch.json", `  "arch": "amd64",`+"\n")
	remote := gitBare(t)
	cmd = exec.Command(auditorBin, "send", "--repo", remote, "--record", withArch, "--image", eif)
	gitIdentity(cmd)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("send with arch: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("arch: amd64")) {
		t.Errorf("send must print the record's arch:\n%s", out)
	}
	if !bytes.Contains(out, []byte("sent ")) {
		t.Errorf("send must report the push:\n%s", out)
	}
	// 보내진 파일에도 arch 가 실려 있다 — 받는 쪽(Vendor)이 읽는 것은 이 파일이다.
	pushed, err := exec.Command("git", "-C", remote, "show", "HEAD:measurement-record.json").CombinedOutput()
	if err != nil {
		t.Fatalf("read pushed record: %v\n%s", err, pushed)
	}
	if !bytes.Contains(pushed, []byte(`"arch": "amd64"`)) {
		t.Errorf("pushed measurement-record.json lacks arch:\n%s", pushed)
	}
}
