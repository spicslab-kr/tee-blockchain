// Tools shared by the e2e tests under cmd: building binaries, finding the repository root, starting ra-serve with a fixture.
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

// build builds cmd/<pkg> into a temporary directory with go build.
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

// startServe starts ra-serve from the vendor binary with a fixture extractor and returns the address it announces.
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
