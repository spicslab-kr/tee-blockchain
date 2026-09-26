// C2 를 실제 CLI 로 — anvil(로컬 EVM)에 실제로 배포·등록해 두고, tb-auditor · tb-user lookup 바이너리로
// 조회한다. cmd/register_e2e_test.go(C1) 의 배포·anvil 헬퍼(findAnvil · buildBin · writeKeyFile 등)를 그대로
// 쓴다 — 같은 패키지(cmd_test)라 다시 안 만든다.
package cmd_test

import (
	"encoding/hex"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestLookup_E2E_FindAddressAndRead(t *testing.T) {
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

	// 컨트랙트 하나에 서명자 하나 — Auditor 것과 Vendor 것을 따로 올린다.
	deploy := exec.Command(auditorBin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--approver", auditorAddr)
	out, err := deploy.CombinedOutput()
	if err != nil {
		t.Fatalf("registry-deploy: %v\n%s", err, out)
	}
	contract := string(deployedAtRe.FindSubmatch(out)[1])
	vDeploy := exec.Command(vendorBin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--approver", vendorAddr)
	vOut, err := vDeploy.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor registry-deploy: %v\n%s", err, vOut)
	}
	vContract := string(deployedAtRe.FindSubmatch(vOut)[1])

	measurements := "PCR0=" + strings.Repeat("aa", 48) + ",PCR1=" + strings.Repeat("bb", 48) + ",PCR2=" + strings.Repeat("cc", 48)
	reg := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements, "--service", "lookup-e2e", "--version", "2.0.0", "--platform", "aws-nitro")
	regOut, err := reg.CombinedOutput()
	if err != nil {
		t.Fatalf("register: %v\n%s", err, regOut)
	}
	key := string(keyLineRe.FindSubmatch(regOut)[1])

	// Vendor 도 같은 측정값을 자기 컨트랙트에 등록 — 컨트랙트가 다르니 조회도 각자 한다.
	vReg := exec.Command(vendorBin, "register", "--rpc", rpc, "--contract", vContract, "--key-file", vendorFile,
		"--measurements", measurements, "--service", "lookup-e2e", "--version", "2.0.0", "--platform", "aws-nitro")
	vRegOut, err := vReg.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor register: %v\n%s", err, vRegOut)
	}
	vendorKeyHex := string(keyLineRe.FindSubmatch(vRegOut)[1])
	if vendorKeyHex == key {
		t.Fatal("auditor 와 vendor 의 key 가 같으면 안 된다 (서명자가 다르다)")
	}

	// 측정값으로 찾기 — auditor 컨트랙트에서. 서명자가 하나라 key 도 하나, auditor 것이어야 한다.
	byMeasurements := exec.Command(auditorBin, "lookup", "--rpc", rpc, "--contract", contract, "--measurements", measurements)
	lmOut, err := byMeasurements.CombinedOutput()
	if err != nil {
		t.Fatalf("lookup --measurements: %v\n%s", err, lmOut)
	}
	if !strings.Contains(string(lmOut), "found 1 key(s)") || !strings.Contains(string(lmOut), key) {
		t.Fatalf("auditor 컨트랙트에선 auditor 의 key 하나가 나와야 한다:\n%s", lmOut)
	}
	if strings.Contains(string(lmOut), vendorKeyHex) {
		t.Fatalf("auditor 컨트랙트 조회에 vendor 의 key 가 섞였다:\n%s", lmOut)
	}
	if !strings.Contains(string(lmOut), "lookup-e2e 2.0.0 (aws-nitro)") {
		t.Errorf("서비스 메타데이터가 안 보인다:\n%s", lmOut)
	}
	// 같은 측정값을 vendor 컨트랙트에서 찾으면 vendor 의 key 하나.
	vByMeasurements := exec.Command(vendorBin, "lookup", "--rpc", rpc, "--contract", vContract, "--measurements", measurements)
	vlmOut, err := vByMeasurements.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor lookup --measurements: %v\n%s", err, vlmOut)
	}
	if !strings.Contains(string(vlmOut), "found 1 key(s)") || !strings.Contains(string(vlmOut), vendorKeyHex) {
		t.Fatalf("vendor 컨트랙트에선 vendor 의 key 하나가 나와야 한다:\n%s", vlmOut)
	}

	// vendor 쪽 key 로 직접 조회 — signer 가 vendor 주소여야 한다.
	vByKey := exec.Command(vendorBin, "lookup", "--rpc", rpc, "--contract", vContract, "--key", vendorKeyHex)
	vbkOut, err := vByKey.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor lookup --key: %v\n%s", err, vbkOut)
	}
	if !strings.Contains(string(vbkOut), vendorAddr) {
		t.Errorf("vendor 등록의 signer 가 vendor 주소가 아니다:\n%s", vbkOut)
	}

	// key 로 바로 읽기 — user 로 (등록 권한이 없는 쪽도 조회는 된다).
	byKey := exec.Command(userBin, "lookup", "--rpc", rpc, "--contract", contract, "--key", key)
	bkOut, err := byKey.CombinedOutput()
	if err != nil {
		t.Fatalf("user lookup --key: %v\n%s", err, bkOut)
	}
	if !strings.Contains(string(bkOut), auditorAddr) {
		t.Errorf("signer 가 auditor 주소가 아니다:\n%s", bkOut)
	}
	hashRe := regexp.MustCompile(`measurement hash\s+([0-9a-f]{64})`)
	m := hashRe.FindSubmatch(bkOut)
	if m == nil {
		t.Fatalf("measurement hash 를 못 찾음:\n%s", bkOut)
	}
	if _, err := hex.DecodeString(string(m[1])); err != nil {
		t.Errorf("measurement hash 가 hex 가 아니다: %s", m[1])
	}

	// 등록되지 않은 측정값 — 빈 결과.
	freshMeasurements := "PCR0=" + strings.Repeat("11", 48) + ",PCR1=" + strings.Repeat("22", 48) + ",PCR2=" + strings.Repeat("33", 48)
	notFound := exec.Command(userBin, "lookup", "--rpc", rpc, "--contract", contract, "--measurements", freshMeasurements)
	nfOut, err := notFound.CombinedOutput()
	if err != nil {
		t.Fatalf("lookup (등록 안 된 측정값): %v\n%s", err, nfOut)
	}
	if !strings.Contains(string(nfOut), "등록된 것이 없다") {
		t.Errorf("등록 안 된 측정값인데 뭔가 찾았다:\n%s", nfOut)
	}

	// 등록되지 않은 key — 에러.
	badKey := exec.Command(userBin, "lookup", "--rpc", rpc, "--contract", contract, "--key", "0x"+strings.Repeat("00", 32))
	if out, err := badKey.CombinedOutput(); err == nil {
		t.Fatalf("등록 안 된 key 조회가 성공하면 안 된다:\n%s", out)
	}
}
