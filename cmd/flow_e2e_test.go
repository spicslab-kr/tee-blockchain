// flow 를 실제 CLI 로 — 사람이 손으로 T1→T2→C1→C2→M2 를 따로 치던 걸 명령 하나로 묶은 것.
// anvil 이 PATH 에 없으면 건너뛴다.
package cmd_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlow_E2E_AuditorVendorUser(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stop := startAnvil(t)
	defer stop()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
	auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)
	vendorFile := writeKeyFile(t, keys, "vendor.key", vendorKey)

	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	userBin := buildBin(t, bins, "./cmd/user")

	deploy := exec.Command(auditorBin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--auditor", auditorAddr, "--vendor", vendorAddr)
	out, err := deploy.CombinedOutput()
	if err != nil {
		t.Fatalf("registry-deploy: %v\n%s", err, out)
	}
	m := deployedAtRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("컨트랙트 주소를 못 찾음:\n%s", out)
	}
	contract := string(m[1])

	fixture := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.cbor")
	fixtureNonce := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.nonce")
	teeAddr, stopServe := startServe(t, vendorBin, fixture)
	defer stopServe()

	// fixture 응답기는 보낸 nonce 에 안 묶인다(FixtureExtractor.Extract 가 nonce 를 무시) — ra-parse 는
	// 그래서 기본값(ra-request 가 방금 보낸 nonce)이 아니라 fixture 가 실제로 들고 있는 nonce 를 받아야 한다.
	work := t.TempDir()
	aFlow := exec.Command(auditorBin, "flow",
		"--tee", teeAddr, "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--service", "flow-e2e", "--version", "1.0.0", "--nonce", fixtureNonce, "--out", filepath.Join(work, "auditor"))
	aOut, err := aFlow.CombinedOutput()
	if err != nil {
		t.Fatalf("auditor flow: %v\n%s", err, aOut)
	}
	if !strings.Contains(string(aOut), "PASS") {
		t.Fatalf("auditor flow 는 self-check PASS 를 찍어야 한다:\n%s", aOut)
	}
	aKey := keyLineRe.FindSubmatch(aOut)
	if aKey == nil {
		t.Fatalf("auditor flow 등록 key 를 못 찾음:\n%s", aOut)
	}

	// Vendor: 같은 TEE 에서 자기도 RA 요청해서 등록 — Auditor 와는 다른 key(서명자가 다르므로).
	vFlow := exec.Command(vendorBin, "flow",
		"--tee", teeAddr, "--rpc", rpc, "--contract", contract, "--key-file", vendorFile,
		"--service", "flow-e2e", "--version", "1.0.0", "--nonce", fixtureNonce, "--out", filepath.Join(work, "vendor"))
	vOut, err := vFlow.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor flow: %v\n%s", err, vOut)
	}
	if !strings.Contains(string(vOut), "PASS") {
		t.Fatalf("vendor flow 는 self-check PASS 를 찍어야 한다:\n%s", vOut)
	}
	vKey := keyLineRe.FindSubmatch(vOut)
	if vKey == nil || string(vKey[1]) == string(aKey[1]) {
		t.Fatalf("vendor flow 의 등록 key 가 auditor 것과 같거나 안 잡힘:\n%s", vOut)
	}

	// User: 처음부터 끝까지 한 번에 — request → parse → lookup → compare. 둘 다 등록했으니 PASS.
	uFlow := exec.Command(userBin, "flow", "--tee", teeAddr, "--rpc", rpc, "--contract", contract,
		"--nonce", fixtureNonce, "--out", filepath.Join(work, "user"))
	uOut, err := uFlow.CombinedOutput()
	if err != nil {
		t.Fatalf("user flow: %v\n%s", err, uOut)
	}
	if !strings.Contains(string(uOut), "신뢰할 수 있다") {
		t.Fatalf("user flow 는 둘 다 등록된 뒤엔 신뢰할 수 있다고 해야 한다:\n%s", uOut)
	}
}

// 아무도 등록 안 한 측정값을 내는 TEE 에 User flow 를 돌리면 — 조회에 아무것도 없으니 exit 1.
func TestFlow_E2E_User_NothingRegistered(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stop := startAnvil(t)
	defer stop()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)

	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	userBin := buildBin(t, bins, "./cmd/user")

	deploy := exec.Command(auditorBin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--auditor", auditorAddr, "--vendor", vendorAddr)
	out, err := deploy.CombinedOutput()
	if err != nil {
		t.Fatalf("registry-deploy: %v\n%s", err, out)
	}
	m := deployedAtRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("컨트랙트 주소를 못 찾음:\n%s", out)
	}
	contract := string(m[1])

	fixture := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.cbor")
	fixtureNonce := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.nonce")
	teeAddr, stopServe := startServe(t, vendorBin, fixture)
	defer stopServe()

	uFlow := exec.Command(userBin, "flow", "--tee", teeAddr, "--rpc", rpc, "--contract", contract,
		"--nonce", fixtureNonce, "--out", t.TempDir())
	out, err = uFlow.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("아무도 등록 안 했으면 exit 1 이어야 한다: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "등록한 사람이 없다") {
		t.Fatalf("이유가 명확해야 한다:\n%s", out)
	}
}
