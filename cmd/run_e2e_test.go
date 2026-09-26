// run 을 실제 CLI 로 — 사람이 손으로 T1→T2→C1→C2→M2 를 따로 치던 걸 명령 하나로 묶은 것.
// 컨트랙트 하나 = 승인자 하나라, Auditor 와 Vendor 는 각자 자기 컨트랙트에 등록한다.
// anvil 이 PATH 에 없으면 건너뛴다.
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixture-my-web.cbor 가 실제로 들고 있는 PCR — nitro-cli build-enclave 가 그때 낸 값 (parse/aws/measure_test.go 와 같다).
const (
	fixturePCR0 = "a0099c1cd2af380adf19601ad09aa49380fc8c3c78e875ac9fe958fc6a836a8d3c30ca5ff5d66c9dc7293cd30ce67fb1"
	fixturePCR1 = "4b4d5b3661b3efc12920900c80e126e4ce783c522de6c02a2a5bf7af3a2b9327b86776f188e4be1c1c404a129dbda493"
	fixturePCR2 = "62f677d0dea159ce42932fa7cc14b885d53d295136f74347a8572ba5f48a36e82485055108bd4f1fa2a3302ab64424cf"
)

// writeBuildMeasurements — build 가 out/measurements.json 에 남기는 것과 같은 nitro-cli 형식.
func writeBuildMeasurements(t *testing.T, dir, name, pcr0, pcr1, pcr2 string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	body := `{"Measurements":{"HashAlgorithm":"Sha384 { ... }","PCR0":"` + pcr0 + `","PCR1":"` + pcr1 + `","PCR2":"` + pcr2 + `"}}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// lastKey — keyLineRe 는 "key ... 에서 가져옴"(service/version 추론 로그)에도 걸린다. register 가 실제로
// 등록한 key 는 항상 그 뒤, 출력의 마지막 매치다.
func lastKey(out []byte) []byte {
	m := keyLineRe.FindAllSubmatch(out, -1)
	if m == nil {
		return nil
	}
	return m[len(m)-1][1]
}

func deployFor(t *testing.T, bin, rpc, deployerFile, approver string) string {
	t.Helper()
	out, err := exec.Command(bin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--approver", approver).CombinedOutput()
	if err != nil {
		t.Fatalf("registry-deploy: %v\n%s", err, out)
	}
	m := deployedAtRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("컨트랙트 주소를 못 찾음:\n%s", out)
	}
	return string(m[1])
}

func TestRun_E2E_AuditorVendorUser(t *testing.T) {
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

	auditorContract := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)
	vendorContract := deployFor(t, vendorBin, rpc, deployerFile, vendorAddr)

	fixture := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.cbor")
	fixtureNonce := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.nonce")
	teeAddr, stopServe := startServe(t, vendorBin, fixture)
	defer stopServe()

	// fixture 응답기는 보낸 nonce 에 안 묶인다(FixtureExtractor.Extract 가 nonce 를 무시) — ra-parse 는
	// 그래서 기본값(ra-request 가 방금 보낸 nonce)이 아니라 fixture 가 실제로 들고 있는 nonce 를 받아야 한다.
	work := t.TempDir()
	goodBuild := writeBuildMeasurements(t, work, "build-good.json", fixturePCR0, fixturePCR1, fixturePCR2)
	badBuild := writeBuildMeasurements(t, work, "build-bad.json", strings.Repeat("ab", 48), fixturePCR1, fixturePCR2)

	// Auditor, 검사 FAIL: 내가 빌드한 값(가짜)과 TEE 가 증명한 값이 다르면 등록하지 않고 실패한다.
	aBad := exec.Command(auditorBin, "run",
		"--tee", teeAddr, "--measurements", badBuild, "--rpc", rpc, "--contract", auditorContract, "--key-file", auditorFile,
		"--service", "run-e2e", "--version", "1.0.0", "--nonce", fixtureNonce, "--out", filepath.Join(work, "auditor-bad"))
	aBadOut, err := aBad.CombinedOutput()
	if err == nil {
		t.Fatalf("빌드값이 TEE 값과 다르면 run 은 실패해야 한다:\n%s", aBadOut)
	}
	if !strings.Contains(string(aBadOut), "등록하지 않는다") || !strings.Contains(string(aBadOut), "PCR0") {
		t.Fatalf("검사 FAIL 의 이유(PCR0)와 '등록하지 않는다'가 찍혀야 한다:\n%s", aBadOut)
	}
	if strings.Contains(string(aBadOut), "registered") {
		t.Fatalf("검사에 실패했는데 등록이 됐다:\n%s", aBadOut)
	}

	// Auditor, 검사 PASS: 빌드값 = TEE 값 → 빌드값을 등록하고 self-check.
	aRun := exec.Command(auditorBin, "run",
		"--tee", teeAddr, "--measurements", goodBuild, "--rpc", rpc, "--contract", auditorContract, "--key-file", auditorFile,
		"--service", "run-e2e", "--version", "1.0.0", "--nonce", fixtureNonce, "--out", filepath.Join(work, "auditor"))
	aOut, err := aRun.CombinedOutput()
	if err != nil {
		t.Fatalf("auditor run: %v\n%s", err, aOut)
	}
	if !strings.Contains(string(aOut), "== 검사") || strings.Count(string(aOut), "PASS") < 2 {
		t.Fatalf("auditor run 은 검사 PASS 와 self-check PASS 를 둘 다 찍어야 한다:\n%s", aOut)
	}
	aKey := lastKey(aOut)
	if aKey == nil {
		t.Fatalf("auditor run 등록 key 를 못 찾음:\n%s", aOut)
	}

	// Vendor: 같은 TEE 에서 자기도 RA 요청해서 자기 컨트랙트에 등록 (흐름2). 자기 컨트랙트엔 아직 아무것도
	// 없으니 service/version 은 직접 준다.
	vRun := exec.Command(vendorBin, "run",
		"--tee", teeAddr, "--rpc", rpc, "--contract", vendorContract, "--key-file", vendorFile,
		"--service", "run-e2e", "--version", "1.0.0", "--nonce", fixtureNonce, "--out", filepath.Join(work, "vendor"))
	vOut, err := vRun.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor run: %v\n%s", err, vOut)
	}
	if !strings.Contains(string(vOut), "PASS") {
		t.Fatalf("vendor run 은 self-check PASS 를 찍어야 한다:\n%s", vOut)
	}
	vKey := lastKey(vOut)
	if vKey == nil || string(vKey) == string(aKey) {
		t.Fatalf("vendor run 의 등록 key 가 auditor 것과 같거나 안 잡힘:\n%s", vOut)
	}

	// Vendor 키로 Auditor 컨트랙트에 등록하려 하면 거부된다 — 컨트랙트 하나 = 승인자 하나.
	vWrong := exec.Command(vendorBin, "run",
		"--measurements", goodBuild, "--rpc", rpc, "--contract", auditorContract, "--key-file", vendorFile,
		"--service", "run-e2e", "--version", "1.0.0")
	if out, err := vWrong.CombinedOutput(); err == nil {
		t.Fatalf("Vendor 키는 Auditor 컨트랙트의 승인자가 아니다 — 실패해야 한다:\n%s", out)
	}

	// User: 처음부터 끝까지 한 번에 — request → parse → lookup → compare. 컨트랙트마다 따로 본다.
	for _, contract := range []string{auditorContract, vendorContract} {
		uRun := exec.Command(userBin, "run", "--tee", teeAddr, "--rpc", rpc, "--contract", contract,
			"--nonce", fixtureNonce, "--out", t.TempDir())
		uOut, err := uRun.CombinedOutput()
		if err != nil {
			t.Fatalf("user run (%s): %v\n%s", contract, err, uOut)
		}
		if !strings.Contains(string(uOut), "신뢰할 수 있다") {
			t.Fatalf("user run 은 등록된 컨트랙트에선 신뢰할 수 있다고 해야 한다:\n%s", uOut)
		}
	}
}

// 아무도 등록 안 한 측정값을 내는 TEE 에 User run 을 돌리면 — 조회에 아무것도 없으니 exit 1.
func TestRun_E2E_User_NothingRegistered(t *testing.T) {
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

	contract := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)

	fixture := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.cbor")
	fixtureNonce := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.nonce")
	teeAddr, stopServe := startServe(t, vendorBin, fixture)
	defer stopServe()

	uRun := exec.Command(userBin, "run", "--tee", teeAddr, "--rpc", rpc, "--contract", contract,
		"--nonce", fixtureNonce, "--out", t.TempDir())
	out, err := uRun.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("아무도 등록 안 했으면 exit 1 이어야 한다: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "등록한 사람이 없다") {
		t.Fatalf("이유가 명확해야 한다:\n%s", out)
	}
}
