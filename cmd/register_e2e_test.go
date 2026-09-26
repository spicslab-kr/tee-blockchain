// C1 을 실제 CLI 로 — tb-auditor · tb-vendor 바이너리를 빌드해 exec 로 띄우고, anvil(로컬 EVM)에 진짜 RPC 로
// registry-deploy · register 를 보낸다. internal/chain/register/ether_test.go 는 Go 바인딩으로 시뮬레이션
// 백엔드를 직접 부르지만, register 명령은 --rpc 로 실제 JSON-RPC 엔드포인트에 붙는 구조라 그걸로는 CLI 배선까지는
// 못 본다. anvil 이 PATH 에 없으면 건너뛴다.
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

// anvil 의 잘 알려진 개발용 키 (test test test ... junk 니모닉). 비밀값이 아니다 — anvil 을 그냥 띄우면 항상 이 키다.
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

// findAnvil — PATH 에서 찾고, 없으면 foundryup 의 기본 설치 위치(~/.foundry/bin)를 본다.
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

// startAnvil — 로컬 EVM 을 띄우고 RPC 주소를 돌려준다. anvil 을 못 찾으면 스킵한다.
func startAnvil(t *testing.T) (rpc string, stop func()) {
	t.Helper()
	anvilPath, err := findAnvil()
	if err != nil {
		t.Skip("anvil not found (foundry) — skipping chain/register cmd e2e")
	}
	// 0 포트는 anvil 이 auto-assign 하지 않으니, 비어 있는 포트를 하나 빌려 쓴다.
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
	// --silent 라도 리슨 메시지는 나온다. 못 찾아도 포트를 아니까 재시도로 대체한다.
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

	// 1. 컨트랙트 배포 — 컨트랙트 하나에 서명자 하나. 역할마다 자기 것을 올린다. 트랜잭션은 누가 보내도 된다,
	// 여기서는 deployer 키로.
	deployFor := func(bin, approver string) string {
		deploy := exec.Command(bin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--approver", approver)
		out, err := deploy.CombinedOutput()
		if err != nil {
			t.Fatalf("registry-deploy: %v\n%s", err, out)
		}
		m := deployedAtRe.FindSubmatch(out)
		if m == nil {
			t.Fatalf("컨트랙트 주소를 못 찾음:\n%s", out)
		}
		if !strings.Contains(string(out), "approver "+approver) {
			t.Fatalf("배포 출력에 approver 가 안 보임:\n%s", out)
		}
		return string(m[1])
	}
	contract := deployFor(auditorBin, auditorAddr)
	vContract := deployFor(vendorBin, vendorAddr)
	t.Logf("registries deployed at %s (auditor) and %s (vendor)", contract, vContract)
	var out []byte
	var err error

	// 2. Auditor 등록.
	measurements := "PCR0=" + strings.Repeat("aa", 48) + ",PCR1=" + strings.Repeat("bb", 48) + ",PCR2=" + strings.Repeat("cc", 48)
	aReg := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro")
	aOut, err := aReg.CombinedOutput()
	if err != nil {
		t.Fatalf("auditor register: %v\n%s", err, aOut)
	}
	aKey := keyLineRe.FindSubmatch(aOut)
	if aKey == nil {
		t.Fatalf("auditor 등록 key 를 못 찾음:\n%s", aOut)
	}

	// 3. 같은 측정값을 Vendor 도 자기 컨트랙트에 등록 — 다른 key 가 나와야 한다 (서명자가 다르므로).
	vReg := exec.Command(vendorBin, "register", "--rpc", rpc, "--contract", vContract, "--key-file", vendorFile,
		"--measurements", measurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro")
	vOut, err := vReg.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor register: %v\n%s", err, vOut)
	}
	vKey := keyLineRe.FindSubmatch(vOut)
	if vKey == nil {
		t.Fatalf("vendor 등록 key 를 못 찾음:\n%s", vOut)
	}
	if string(aKey[1]) == string(vKey[1]) {
		t.Fatal("auditor 와 vendor 의 등록 key 가 같으면 안 된다 (서명자가 다르다)")
	}

	// 4. 같은 것을 auditor 로 다시 — 거부, exit 5.
	again := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro")
	out, err = again.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 5 {
		t.Fatalf("want exit 5 (already registered), got %v\n%s", err, out)
	}

	// 5. 승인자가 아닌 키로 — 거부, exit 4. 새 측정값으로 (이미 등록됨과 안 섞이게).
	freshMeasurements := "PCR0=" + strings.Repeat("dd", 48) + ",PCR1=" + strings.Repeat("ee", 48) + ",PCR2=" + strings.Repeat("ff", 48)
	stranger := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", strangerFile,
		"--measurements", freshMeasurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro")
	out, err = stranger.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 4 {
		t.Fatalf("want exit 4 (not an approver), got %v\n%s", err, out)
	}

	// 5b. Vendor 키로 Auditor 컨트랙트에 — 승인자가 아니라 거부, exit 4. 컨트랙트 하나에 서명자 하나라는 뜻이 이것.
	cross := exec.Command(vendorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", vendorFile,
		"--measurements", freshMeasurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro")
	out, err = cross.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 4 {
		t.Fatalf("vendor key on the auditor contract: want exit 4, got %v\n%s", err, out)
	}

	// 6. 이미 지난 만료 — 거부, exit 6.
	expired := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", freshMeasurements, "--service", "e2e-svc", "--version", "1.0.0", "--platform", "aws-nitro", "--expires", "-1h")
	out, err = expired.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 6 {
		t.Fatalf("want exit 6 (expiry passed), got %v\n%s", err, out)
	}
}
