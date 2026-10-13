// register, through the real CLI: build the att-auditor and att-vendor binaries, run them with exec, and send registry-deploy and register
// over real RPC to anvil (a local EVM). internal/chain/register/ether_test.go calls the simulated backend directly
// through the Go bindings, but the register command attaches to a real JSON-RPC endpoint with --rpc, so that cannot cover the CLI wiring.
// Skipped if anvil is not on PATH.
package cmd_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// anvil's well-known development key (the test test test ... junk mnemonic). Not a secret: a plain anvil start always has this key.
const (
	deployerAddr = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	deployerKey  = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	auditorAddr  = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
	auditorKey   = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	vendorAddr   = "0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC"
	vendorKey    = "5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"
	strangerKey  = "7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6"
)

func buildBin(t *testing.T, dir, pkg string) string {
	t.Helper()
	bin := filepath.Join(dir, filepath.Base(pkg))
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = repoRootReg(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

func repoRootReg(t *testing.T) string {
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

// findAnvil looks on PATH, and if absent, at foundryup's default install location (~/.foundry/bin).
func findAnvil() (string, error) {
	if p, err := exec.LookPath("anvil"); err == nil {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := "anvil"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(home, ".foundry", "bin", name)
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

// startAnvil starts a local EVM and returns the RPC address. Skips if anvil cannot be found.
func startAnvil(t *testing.T) (rpc string, stop func()) {
	t.Helper()
	anvilPath, err := findAnvil()
	if err != nil {
		t.Skip("anvil not found (foundry) — skipping chain/register cmd e2e")
	}
	// anvil does not auto-assign port 0, so borrow a free port.
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, anvilPath, "--port", fmt.Sprint(port), "--silent")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	// the listen message is printed even with --silent. If it cannot be found, we know the port, so fall back to retrying.
	done := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
		}
		close(done)
	}()
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !waitForRPC(addr, 10*time.Second) {
		cancel()
		t.Fatal("anvil did not start listening in time")
	}
	return addr, func() { cancel(); _ = cmd.Wait(); <-done }
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForRPC(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://"), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func writeKeyFile(t *testing.T, dir, name, hexKey string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(hexKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var deployedAtRe = regexp.MustCompile(`Registry deployed at (0x[0-9a-fA-F]{40})`)
var keyLineRe = regexp.MustCompile(`key\s+(0x[0-9a-fA-F]{64})`)

func TestRegister_E2E_DeployAndRegister(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stop := startAnvil(t)
	defer stop()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
	auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)
	vendorFile := writeKeyFile(t, keys, "vendor.key", vendorKey)
	strangerFile := writeKeyFile(t, keys, "stranger.key", strangerKey)

	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")

	// 1. deploy the contract: one signer per contract, each role deploys its own. Anyone may send the transaction,
	// here it is the deployer key.
	deployFor := func(bin, approver string) string {
		deploy := exec.Command(bin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--approver", approver)
		out, err := deploy.CombinedOutput()
		if err != nil {
			t.Fatalf("registry-deploy: %v\n%s", err, out)
		}
		m := deployedAtRe.FindSubmatch(out)
		if m == nil {
			t.Fatalf("contract address not found:\n%s", out)
		}
		if !strings.Contains(string(out), "approver "+approver) {
			t.Fatalf("approver not shown in the deploy output:\n%s", out)
		}
		return string(m[1])
	}
	contract := deployFor(auditorBin, auditorAddr)
	vContract := deployFor(vendorBin, vendorAddr)
	t.Logf("registries deployed at %s (auditor) and %s (vendor)", contract, vContract)
	var out []byte
	var err error

	// 2. Auditor registers. --arch is carried in the on-chain Entry, so the lookup below must read it back as is.
	measurements := "PCR0=" + strings.Repeat("aa", 48) + ",PCR1=" + strings.Repeat("bb", 48) + ",PCR2=" + strings.Repeat("cc", 48)
	aReg := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro", "--arch", "arm64")
	aOut, err := aReg.CombinedOutput()
	if err != nil {
		t.Fatalf("auditor register: %v\n%s", err, aOut)
	}
	aKey := keyLineRe.FindSubmatch(aOut)
	if aKey == nil {
		t.Fatalf("auditor registration key not found:\n%s", aOut)
	}

	// 3. the Vendor registers the same measurements on its own contract: a different key must come out (the signers differ).
	vReg := exec.Command(vendorBin, "register", "--rpc", rpc, "--contract", vContract, "--key-file", vendorFile,
		"--measurements", measurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro", "--arch", "amd64")
	vOut, err := vReg.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor register: %v\n%s", err, vOut)
	}
	vKey := keyLineRe.FindSubmatch(vOut)
	if vKey == nil {
		t.Fatalf("vendor registration key not found:\n%s", vOut)
	}
	if string(aKey[1]) == string(vKey[1]) {
		t.Fatal("the auditor and vendor registration keys must not be equal (the signers differ)")
	}

	// 3b. the chain returns which architecture a registration is for: the --arch given at registration must show as is in the lookup output.
	// arch is not in the key (same measurements, same key), so the lookup need not be given an arch.
	for _, c := range []struct{ bin, contract, key, arch string }{
		{auditorBin, contract, string(aKey[1]), "arm64"},
		{vendorBin, vContract, string(vKey[1]), "amd64"},
	} {
		look := exec.Command(c.bin, "lookup", "--rpc", rpc, "--contract", c.contract, "--key", c.key)
		out, err := look.CombinedOutput()
		if err != nil {
			t.Fatalf("lookup --key %s: %v\n%s", c.key, err, out)
		}
		if want := "e2e-svc 1.0.0 (aws-nitro, " + c.arch + ")"; !strings.Contains(string(out), want) {
			t.Errorf("lookup must print the architecture the chain recorded, lacks %q:\n%s", want, out)
		}
	}

	// 4. the same again as auditor: rejected, exit 5.
	again := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro")
	out, err = again.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 5 {
		t.Fatalf("want exit 5 (already registered), got %v\n%s", err, out)
	}

	// 5. with a key that is not the approver: rejected, exit 4. Use new measurements (so it is not confused with already-registered).
	freshMeasurements := "PCR0=" + strings.Repeat("dd", 48) + ",PCR1=" + strings.Repeat("ee", 48) + ",PCR2=" + strings.Repeat("ff", 48)
	stranger := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", strangerFile,
		"--measurements", freshMeasurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro")
	out, err = stranger.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 4 {
		t.Fatalf("want exit 4 (not an approver), got %v\n%s", err, out)
	}

	// 5b. with the Vendor key on the Auditor contract: rejected because it is not the approver, exit 4. This is what one signer per contract means.
	cross := exec.Command(vendorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", vendorFile,
		"--measurements", freshMeasurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro")
	out, err = cross.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 4 {
		t.Fatalf("vendor key on the auditor contract: want exit 4, got %v\n%s", err, out)
	}

	// 6. an expiry already in the past: rejected, exit 6.
	expired := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", freshMeasurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro", "--expires", "-1h")
	out, err = expired.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 6 {
		t.Fatalf("want exit 6 (expiry passed), got %v\n%s", err, out)
	}
}
