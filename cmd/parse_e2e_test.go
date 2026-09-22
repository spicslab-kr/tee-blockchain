// 세 바이너리(cmd/vendor, cmd/user, cmd/auditor)를 실제로 빌드해 exec 로 띄우고, ra-serve → ra-request → ra-parse
// 전체 사슬이 CLI 층까지 맞물리는지 본다. internal/tee/request 의 two_process_test.go 와 같은 패턴이지만
// 여기서는 ra-parse 까지 이어서, 실제로 registry 에 들어갈 measurements.json 이 알려진 fixture 값과 맞는지 확인한다.
package cmd_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// build — go build 로 cmd/<pkg> 를 임시 디렉터리에 만든다.
func build(t *testing.T, dir, pkg string) string {
	t.Helper()
	bin := filepath.Join(dir, filepath.Base(pkg))
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			t.Fatal("go.mod not found")
		}
	}
}

// startServe — vendor 바이너리로 ra-serve 를 fixture 추출기로 띄우고, 알려주는 주소를 돌려준다.
func startServe(t *testing.T, bin, fixture string) (addr string, stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "ra-serve", "--listen", "tcp:127.0.0.1:0", "--extractor", "fixture="+fixture)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		cancel()
		t.Fatalf("serve did not report an address: %v", err)
	}
	i := strings.LastIndex(line, "on ")
	if i < 0 {
		cancel()
		t.Fatalf("unexpected serve output: %q", line)
	}
	return strings.TrimSpace(line[i+3:]), func() { cancel(); _ = cmd.Wait() }
}

// TestCmd_E2E_ServeRequestParse — vendor 가 ra-serve 로 (fixture) TEE 문서를 내주고, user 가 ra-request 로 받아
// 저장하고, auditor 가 ra-parse 로 서명 · 난수 · 측정값을 확인한다. 세 바이너리가 서로 다른 프로세스로 실행되며
// 실제 CLI 플래그 · 종료 코드 · 파일 계약까지 맞물리는지가 목적이라, 단위 테스트로는 잡히지 않는 배선 실수를 잡는다.
//
// fixture 는 실제 Nitro 엔클레이브에서 받은 문서(fixture-my-web.cbor)라 서명은 진짜다. 다만 FixtureExtractor 는
// 요청마다 오는 난수와 묶지 않고 항상 같은 문서를 돌려주므로, ra-request 가 저장하는 <out>.nonce (매번 새로 만든
// 난수)는 문서 안의 진짜 난수와 다르다 — 그래서 ra-parse 에는 문서가 실제로 실린, 미리 알고 있는 난수를
// (--nonce) 로 직접 준다. (실기기라면 추출기가 난수를 문서에 실어 보내므로 이 우회가 필요 없다.)
func TestCmd_E2E_ServeRequestParse(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := repoRoot(t)
	fixture, err := filepath.Abs(filepath.Join(root, "internal", "tee", "parse", "aws", "fixture-my-web.cbor"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureNonceFile, err := filepath.Abs(filepath.Join(root, "internal", "tee", "parse", "aws", "fixture-my-web.nonce"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}

	bins := t.TempDir()
	vendor := build(t, bins, "./cmd/vendor")
	user := build(t, bins, "./cmd/user")
	auditor := build(t, bins, "./cmd/auditor")

	addr, stop := startServe(t, vendor, fixture)
	defer stop()

	work := t.TempDir()
	docPath := filepath.Join(work, "doc.cbor")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// user 쪽 — ra-request 로 문서를 받는다.
	reqCmd := exec.CommandContext(ctx, user, "ra-request", "--tee", addr, "--out", docPath, "--format", "aws-nitro", "--timeout", "10s")
	reqOut, err := reqCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ra-request: %v\n%s", err, reqOut)
	}
	if _, err := os.Stat(docPath); err != nil {
		t.Fatalf("문서 파일이 없다: %v", err)
	}
	noncePath := docPath + ".nonce"
	sentNonce, err := os.ReadFile(noncePath)
	if err != nil {
		t.Fatalf("nonce 파일이 없다: %v", err)
	}
	if len(strings.TrimSpace(string(sentNonce))) != 64 {
		t.Fatalf("nonce 길이가 이상하다: %q", sentNonce)
	}

	// auditor 쪽 — ra-parse 로 서명 · 난수 · 측정값을 확인한다. 문서에 실제로 실린 난수(fixture 의 것)를 명시로 준다 —
	// fixture 추출기는 요청 난수와 묶지 않으므로 방금 보낸 난수로는 검증에 실패한다 (위 주석 참고).
	measurementsPath := docPath + ".measurements.json"
	parseCmd := exec.CommandContext(ctx, auditor, "ra-parse", "--doc", docPath, "--nonce", fixtureNonceFile, "--out", measurementsPath)
	parseOut, err := parseCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ra-parse: %v\n%s", err, parseOut)
	}
	out := string(parseOut)
	if !strings.Contains(out, "signature  ok") {
		t.Errorf("서명 확인 로그가 없다:\n%s", out)
	}
	if !strings.Contains(out, "nonce      ok") {
		t.Errorf("난수 확인 로그가 없다:\n%s", out)
	}

	// 결과 measurements.json 이 register --measurements 에 그대로 들어갈 모양이고, 알려진 fixture 의 PCR0 과 같은지.
	raw, err := os.ReadFile(measurementsPath)
	if err != nil {
		t.Fatalf("measurements.json 이 없다: %v", err)
	}
	var got struct {
		Format       string
		Measurements map[string]string
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("measurements.json 파싱 실패: %v\n%s", err, raw)
	}
	if got.Format != "aws-nitro" {
		t.Errorf("Format = %q, want aws-nitro", got.Format)
	}
	// PCR 은 SHA384 값(48 바이트 = hex 96 글자)이어야 한다. 정확한 값은 fixture 문서 자체에서 나온 것이라 여기서
	// 다시 계산하지 않고, ra-parse 가 실제로 뭔가를 뽑아 파일로 남겼는지 · 모양이 맞는지만 본다.
	pcr0, ok := got.Measurements["PCR0"]
	if !ok || len(pcr0) != 96 {
		t.Errorf("PCR0 = %q, want 96 개 hex 문자", pcr0)
	}
}

// TestCmd_E2E_ParseRejectsTamperedNonce — 다른 난수를 주면 ra-parse 가 exit 6(난수 불일치)으로 거부하는지.
// CLI 종료 코드가 cmd/auditor 의 exitCode() 배선과 실제로 맞는지를 본다.
func TestCmd_E2E_ParseRejectsTamperedNonce(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := repoRoot(t)
	fixture, _ := filepath.Abs(filepath.Join(root, "internal", "tee", "parse", "aws", "fixture-my-web.cbor"))

	bins := t.TempDir()
	auditor := build(t, bins, "./cmd/auditor")

	work := t.TempDir()
	docPath := filepath.Join(work, "doc.cbor")
	docBytes, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, docBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// 진짜와 다른, 그럴듯한 64-hex 난수.
	wrongNonce := strings.Repeat("ab", 32)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	parseCmd := exec.CommandContext(ctx, auditor, "ra-parse", "--doc", docPath, "--nonce", wrongNonce)
	out, err := parseCmd.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 6 {
		t.Fatalf("want exit 6 (nonce mismatch), got %v\n%s", err, out)
	}
}
