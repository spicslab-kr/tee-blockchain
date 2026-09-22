// M2 를 실제 CLI 로 — anvil(로컬 EVM)에 실제로 배포·등록해 두고, tb-auditor · tb-user compare 바이너리로
// 대조한다. cmd/register_e2e_test.go(C1) 의 헬퍼(findAnvil · buildBin · writeKeyFile · deployedAtRe ·
// keyLineRe 등)를 그대로 쓴다 — 같은 패키지(cmd_test)라 다시 안 만든다.
package cmd_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCompare_E2E_PlainAndChainModes(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stop := startAnvil(t)
	defer stop()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
	auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)

	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	userBin := buildBin(t, bins, "./cmd/user")

	deploy := exec.Command(auditorBin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--auditor", auditorAddr, "--vendor", vendorAddr)
	out, err := deploy.CombinedOutput()
	if err != nil {
		t.Fatalf("registry-deploy: %v\n%s", err, out)
	}
	contract := string(deployedAtRe.FindSubmatch(out)[1])

	measurements := "PCR0=" + strings.Repeat("aa", 48) + ",PCR1=" + strings.Repeat("bb", 48)
	reg := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements, "--service", "compare-e2e", "--version", "1.0.0", "--platform", "aws-nitro")
	regOut, err := reg.CombinedOutput()
	if err != nil {
		t.Fatalf("register: %v\n%s", err, regOut)
	}
	key := string(keyLineRe.FindSubmatch(regOut)[1])

	// 1. 그냥 평문 비교(Compare) — 같은 값.
	same := exec.Command(auditorBin, "compare", "--a", measurements, "--b", measurements)
	out, err = same.CombinedOutput()
	if err != nil {
		t.Fatalf("compare --a --b (같은 값): %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "PASS" {
		t.Fatalf("같은 값인데 PASS 가 아님:\n%s", out)
	}

	// 2. 평문 비교 — 다른 값. exit 1 이어야 한다.
	other := "PCR0=" + strings.Repeat("11", 48) + ",PCR1=" + strings.Repeat("22", 48)
	diff := exec.Command(auditorBin, "compare", "--a", measurements, "--b", other)
	out, err = diff.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("다른 값인데 exit 1 이 아님: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "FAIL") {
		t.Errorf("FAIL 이 안 찍힘:\n%s", out)
	}

	// 3. 체인 모드(Check) — 등록한 값 그대로, user 로 (조회만 하는 쪽도 대조는 된다). --trusted 없이 —
	// 컨트랙트의 AUDITOR·VENDOR 를 자동으로 가져와 써야 한다.
	chainSame := exec.Command(userBin, "compare", "--a", measurements, "--rpc", rpc, "--contract", contract, "--key", key)
	out, err = chainSame.CombinedOutput()
	if err != nil {
		t.Fatalf("compare 체인모드 (같은 값): %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "PASS" {
		t.Fatalf("등록한 값 그대로인데 PASS 가 아님:\n%s", out)
	}

	// 4. 체인 모드 — 다른 값. exit 1, 근거에 해시 불일치가 있어야 한다.
	chainDiff := exec.Command(userBin, "compare", "--a", other, "--rpc", rpc, "--contract", contract, "--key", key)
	out, err = chainDiff.CombinedOutput()
	ee, ok = err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("체인모드, 다른 값인데 exit 1 이 아님: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "측정값 해시") {
		t.Errorf("해시 불일치 근거가 안 보임:\n%s", out)
	}

	// 5. 체인 모드 — 신뢰 목록을 컨트랙트와 무관한 주소로 강제. 값은 맞아도 서명자 불신으로 실패해야 한다.
	untrusted := exec.Command(userBin, "compare", "--a", measurements, "--rpc", rpc, "--contract", contract, "--key", key,
		"--trusted", "0x0000000000000000000000000000000000000001")
	out, err = untrusted.CombinedOutput()
	ee, ok = err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("--trusted 강제로 신뢰 안 되는데 exit 1 이 아님: %v\n%s", err, out)
	}

	// 6. 실수로 --b 와 --key 를 같이 주면 — 하나를 조용히 무시하지 않고 바로 에러여야 한다 (exit 2, flag 검증 실패).
	both := exec.Command(userBin, "compare", "--a", measurements, "--b", measurements, "--rpc", rpc, "--contract", contract, "--key", key)
	out, err = both.CombinedOutput()
	if err == nil {
		t.Fatalf("--b 와 --key 를 같이 줬는데 성공함 (하나가 조용히 무시된 것):\n%s", out)
	}
	if !strings.Contains(string(out), "같이 주면 안 된다") {
		t.Errorf("어느 쪽을 골라야 하는지 에러 메시지가 안 보임:\n%s", out)
	}

	// 7. --trusted 만 주고 --key(체인 모드 전체)는 빠뜨리면 — 조용히 무시되지 않고 에러여야 한다.
	trustedNoKey := exec.Command(userBin, "compare", "--a", measurements, "--trusted", "0x0000000000000000000000000000000000000001")
	out, err = trustedNoKey.CombinedOutput()
	if err == nil {
		t.Fatalf("--trusted 만 주고 --key 를 안 줬는데 성공함 (--trusted 가 조용히 무시된 것):\n%s", out)
	}
	if !strings.Contains(string(out), "체인 모드 전용") {
		t.Errorf("--trusted 전용이라는 에러 메시지가 안 보임:\n%s", out)
	}
}
