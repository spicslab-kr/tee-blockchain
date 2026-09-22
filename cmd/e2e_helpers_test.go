// cmd 밑 e2e 테스트들이 공유하는 도구 — 바이너리 빌드, 저장소 루트 찾기, fixture 로 ra-serve 띄우기.
// request_e2e_test.go(T1) 와 parse_e2e_test.go(T2) 가 원래 각자 들고 있던 걸 하나로 모았다.
package cmd_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
