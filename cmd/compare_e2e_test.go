// compare, through the real CLI: deploy and register for real on anvil (a local EVM), then compare with the tb-auditor and tb-user compare binaries.
// It reuses the helpers of cmd/register_e2e_test.go (findAnvil, buildBin, writeKeyFile, deployedAtRe,
// keyLineRe, ...) as they are: same package (cmd_test), so they are not rebuilt.
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

	deploy := exec.Command(auditorBin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--approver", auditorAddr)
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

	// 1. plain comparison (Compare): same values.
	same := exec.Command(auditorBin, "compare", "--a", measurements, "--b", measurements)
	out, err = same.CombinedOutput()
	if err != nil {
		t.Fatalf("compare --a --b (same values): %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "PASS" {
		t.Fatalf("same values but not PASS:\n%s", out)
	}

	// 2. plain comparison: different values. Must exit 1.
	other := "PCR0=" + strings.Repeat("11", 48) + ",PCR1=" + strings.Repeat("22", 48)
	diff := exec.Command(auditorBin, "compare", "--a", measurements, "--b", other)
	out, err = diff.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("different values but not exit 1: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "FAIL") {
		t.Errorf("FAIL not printed:\n%s", out)
	}

	// 3. chain mode (Check): the registered values as they are, as user (the read-only side can compare too). Without --trusted,
	// it must fetch the contract's APPROVER automatically and use it.
	chainSame := exec.Command(userBin, "compare", "--a", measurements, "--rpc", rpc, "--contract", contract, "--key", key)
	out, err = chainSame.CombinedOutput()
	if err != nil {
		t.Fatalf("compare chain mode (same values): %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "PASS" {
		t.Fatalf("registered values as they are but not PASS:\n%s", out)
	}

	// 4. chain mode: different values. Exit 1, and the reasons must include a hash mismatch.
	chainDiff := exec.Command(userBin, "compare", "--a", other, "--rpc", rpc, "--contract", contract, "--key", key)
	out, err = chainDiff.CombinedOutput()
	ee, ok = err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("chain mode, different values but not exit 1: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "measurement hash") {
		t.Errorf("hash mismatch reason not shown:\n%s", out)
	}

	// 5. chain mode: force the trusted list to an address unrelated to the contract. Even with matching values it must fail because the signer is untrusted.
	untrusted := exec.Command(userBin, "compare", "--a", measurements, "--rpc", rpc, "--contract", contract, "--key", key,
		"--trusted", "0x0000000000000000000000000000000000000001")
	out, err = untrusted.CombinedOutput()
	ee, ok = err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("--trusted forced to an untrusted signer but not exit 1: %v\n%s", err, out)
	}

	// 6. giving both --b and --key by mistake: one must not be silently ignored, it must be an error right away (exit 2, flag validation failure).
	both := exec.Command(userBin, "compare", "--a", measurements, "--b", measurements, "--rpc", rpc, "--contract", contract, "--key", key)
	out, err = both.CombinedOutput()
	if err == nil {
		t.Fatalf("--b and --key given together but it succeeded (one was silently ignored):\n%s", out)
	}
	if !strings.Contains(string(out), "pick one") {
		t.Errorf("error message does not say which one to choose:\n%s", out)
	}

	// 7. --trusted without --key (chain mode as a whole) must not be silently ignored, it must be an error.
	trustedNoKey := exec.Command(userBin, "compare", "--a", measurements, "--trusted", "0x0000000000000000000000000000000000000001")
	out, err = trustedNoKey.CombinedOutput()
	if err == nil {
		t.Fatalf("--trusted given without --key but it succeeded (--trusted was silently ignored):\n%s", out)
	}
	if !strings.Contains(string(out), "chain mode only") {
		t.Errorf("error message saying --trusted is chain-mode only not shown:\n%s", out)
	}
}
