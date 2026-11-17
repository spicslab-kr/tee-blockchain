package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestCheckResponder_Rules checks the seven rules on hand-made values, so `-short` still covers them
// without compiling anything. Each refusal must name the actual and the expected value and say
// how to rebuild.
func TestCheckResponder_Rules(t *testing.T) {
	want := RequiredToolchain()
	good := ResponderInfo{Go: want, GOOS: "linux", GOARCH: "amd64", Trimpath: true, CGO: "0", SHA256: strings.Repeat("ab", 32)}
	cases := []struct {
		name  string
		info  ResponderInfo
		host  string
		wants []string // substrings the error must contain; empty = must pass
	}{
		{"correct", good, "amd64", nil},
		{"wrong go", func() ResponderInfo { i := good; i.Go = "go1.26.0"; return i }(), "amd64",
			[]string{"built with go1.26.0", "pins " + want, "GO_TOOLCHAIN", "GOTOOLCHAIN=" + want}},
		{"no trimpath", func() ResponderInfo { i := good; i.Trimpath = false; return i }(), "amd64",
			[]string{"without -trimpath", "go build -trimpath -buildvcs=false -ldflags=-buildid="}},
		{"vcs stamped", func() ResponderInfo { i := good; i.VCS = "git"; return i }(), "amd64",
			[]string{"built with version control stamping", "different commit", "uncommitted changes", "go build -trimpath -buildvcs=false -ldflags=-buildid="}},
		{"build id", func() ResponderInfo { i := good; i.BuildID = "Sa2K/eFe5/2iRp/SUjt"; return i }(), "amd64",
			[]string{"carries a Go build ID (Sa2K/eFe5/2iRp/SUjt)", "without -ldflags=-buildid=", "every input of the build", "-ldflags=-buildid= -o att-ra-responder"}},
		{"cgo", func() ResponderInfo { i := good; i.CGO = "1"; return i }(), "amd64",
			[]string{"built with CGO_ENABLED=1", "C compiler", "CGO_ENABLED=0 GOOS=linux"}},
		{"cgo not recorded", func() ResponderInfo { i := good; i.CGO = ""; return i }(), "amd64",
			[]string{"no CGO_ENABLED setting", "CGO_ENABLED=0 GOOS=linux"}},
		{"wrong os", func() ResponderInfo { i := good; i.GOOS = "windows"; return i }(), "amd64",
			[]string{"GOOS=windows", "enclave runs linux", "GOOS=linux"}},
		{"wrong arch", good, "arm64",
			[]string{"GOARCH=amd64", "this instance is arm64", "GOARCH=arm64 go build"}},
		{"empty info (no settings)", ResponderInfo{Go: want}, "amd64",
			[]string{"without -trimpath"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckResponder(c.info, c.host)
			if len(c.wants) == 0 {
				if err != nil {
					t.Fatalf("expected pass, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected refusal")
			}
			for _, w := range c.wants {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error lacks %q:\n%v", w, err)
				}
			}
		})
	}
}

func TestResponderInfo_String(t *testing.T) {
	i := ResponderInfo{Go: "go1.26.1", GOOS: "linux", GOARCH: "amd64", Trimpath: true, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if got, want := i.String(), "responder: att-ra-responder sha256 0123456789abcdef · go1.26.1 linux/amd64 -trimpath"; got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	i.Trimpath = false
	if !strings.HasSuffix(i.String(), "(no -trimpath)") {
		t.Fatalf("no-trimpath line: %q", i.String())
	}
}

func TestReadResponder_NotAGoBinary(t *testing.T) {
	p := filepath.Join(t.TempDir(), "att-ra-responder")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho not a go binary\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ReadResponder(p)
	if err == nil || !strings.Contains(err.Error(), "has no Go build info; is it really att-ra-responder built from ./cmd/ra-responder?") {
		t.Fatalf("expected the no-build-info error, got: %v", err)
	}
	if _, err := ReadResponder(filepath.Join(t.TempDir(), "missing")); err == nil || !os.IsNotExist(err) {
		t.Fatalf("missing file should be a not-exist error, got: %v", err)
	}
}

// buildResponderNoTrimpath is like BuildResponderForArch but deliberately without -trimpath, to
// produce the artifact the check must refuse.
func buildResponderNoTrimpath(t *testing.T, repoRoot, goarch, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-buildvcs=false", "-ldflags=-buildid=", "-o", out, "./cmd/ra-responder")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goarch)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build without -trimpath: %v\n%s", err, o)
	}
}

// buildResponderVCS builds with -trimpath but forces version control stamping (-buildvcs=true), the
// default when -buildvcs is left out inside a git checkout. It reports false when Go cannot stamp
// (for example a source tree with no .git), so the caller can skip.
func buildResponderVCS(t *testing.T, repoRoot, goarch, out string) bool {
	t.Helper()
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=true", "-ldflags=-buildid=", "-o", out, "./cmd/ra-responder")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goarch)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Logf("go build -buildvcs=true failed, so no stamped responder to test (%v): %s", err, o)
		return false
	}
	return true
}

// TestReadResponder_RealBuilds cross-builds real responders and runs the check on the actual
// artifacts: the correct one passes, the same file is refused for another host architecture,
// one built with Go's build ID is refused naming it, and one built without -trimpath is refused naming -trimpath.
func TestReadResponder_RealBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles real binaries; slow")
	}
	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	amd64 := ArchTarget{Name: "x86_64", GOARCH: "amd64"}

	goodPath := filepath.Join(dir, "responder-amd64")
	if err := BuildResponderForArch(context.Background(), repoRoot, amd64, goodPath); err != nil {
		t.Fatal(err)
	}
	info, err := ReadResponder(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Go != RequiredToolchain() {
		t.Errorf("Go = %q, want the pin %q (is the test running under the pinned toolchain?)", info.Go, RequiredToolchain())
	}
	if info.GOOS != "linux" || info.GOARCH != "amd64" || !info.Trimpath || info.VCS != "" || info.CGO != "0" || info.BuildID != "" {
		t.Errorf("unexpected build facts (BuildResponderForArch must use -trimpath -buildvcs=false -ldflags=-buildid= CGO_ENABLED=0): %+v", info)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(info.SHA256) {
		t.Errorf("SHA256 = %q, want 64 hex", info.SHA256)
	}
	if sum, _ := SHA256File(goodPath); sum != info.SHA256 {
		t.Errorf("SHA256 %s differs from SHA256File %s", info.SHA256, sum)
	}
	if err := CheckResponder(info, "amd64"); err != nil {
		t.Errorf("correct responder refused: %v", err)
	}
	err = CheckResponder(info, "arm64")
	if err == nil || !strings.Contains(err.Error(), "GOARCH=amd64") || !strings.Contains(err.Error(), "arm64") {
		t.Errorf("amd64 responder on an arm64 host should be refused naming both: %v", err)
	}

	// the same command without -ldflags=-buildid=: Go writes its build ID, which the check must find and refuse
	withID := filepath.Join(dir, "responder-buildid")
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", withID, "./cmd/ra-responder")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build without -ldflags=-buildid=: %v\n%s", err, o)
	}
	info, err = ReadResponder(withID)
	if err != nil {
		t.Fatal(err)
	}
	if info.BuildID == "" || strings.HasPrefix(info.BuildID, "(") {
		t.Fatalf("a build without -ldflags=-buildid= reports no build ID: %+v", info)
	}
	err = CheckResponder(info, "amd64")
	if err == nil || !strings.Contains(err.Error(), "carries a Go build ID ("+info.BuildID+")") {
		t.Errorf("responder with a build ID should be refused naming it: %v", err)
	}

	noTrim := filepath.Join(dir, "responder-notrimpath")
	buildResponderNoTrimpath(t, repoRoot, "amd64", noTrim)
	info, err = ReadResponder(noTrim)
	if err != nil {
		t.Fatal(err)
	}
	if info.Trimpath {
		t.Fatalf("a build without -trimpath reports Trimpath=true: %+v", info)
	}
	err = CheckResponder(info, "amd64")
	if err == nil || !strings.Contains(err.Error(), "without -trimpath") {
		t.Errorf("responder built without -trimpath should be refused naming -trimpath: %v", err)
	}

	stamped := filepath.Join(dir, "responder-vcs")
	if !buildResponderVCS(t, repoRoot, "amd64", stamped) {
		t.Skip("this source tree cannot be stamped with version control information")
	}
	info, err = ReadResponder(stamped)
	if err != nil {
		t.Fatal(err)
	}
	if info.VCS == "" || !info.Trimpath {
		t.Fatalf("a -trimpath -buildvcs=true build should report vcs and -trimpath: %+v", info)
	}
	err = CheckResponder(info, "amd64")
	if err == nil || !strings.Contains(err.Error(), "built with version control stamping") || !strings.Contains(err.Error(), "-buildvcs=false") {
		t.Errorf("responder stamped with version control should be refused naming it: %v", err)
	}
}

// envWithout returns the environment with every NAME=... entry for the given names removed.
func envWithout(names ...string) []string {
	var env []string
next:
	for _, kv := range os.Environ() {
		for _, n := range names {
			if strings.HasPrefix(kv, n+"=") {
				continue next
			}
		}
		env = append(env, kv)
	}
	return env
}

// TestReadResponder_NativeBuildLinksCgo builds the responder the way the README did before
// CGO_ENABLED=0 was added: for the host's own linux/GOARCH, with -trimpath -buildvcs=false -ldflags=-buildid= and no
// CGO_ENABLED in the environment. On Linux with a C compiler (gcc) Go then links cgo, and on
// 2026-11-15 that gave sha256 a71bedfc... instead of the 5573b538... every cross-build gives. The
// check must refuse that file. Where Go does not enable cgo by itself (no C compiler, or a
// cross-build from Windows or macOS) there is nothing to refuse and the test skips.
func TestReadResponder_NativeBuildLinksCgo(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a real binary; slow")
	}
	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "responder-default-cgo")
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-buildid=", "-o", out, "./cmd/ra-responder")
	cmd.Dir = repoRoot
	cmd.Env = append(envWithout("CGO_ENABLED", "GOOS", "GOARCH"), "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build without CGO_ENABLED: %v\n%s", err, o)
	}
	info, err := ReadResponder(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.CGO != "1" {
		t.Skipf("Go did not enable cgo here (CGO_ENABLED=%q; no C compiler, or not a native linux build), so this machine cannot produce the file to refuse", info.CGO)
	}
	err = CheckResponder(info, runtime.GOARCH)
	if err == nil || !strings.Contains(err.Error(), "built with CGO_ENABLED=1") || !strings.Contains(err.Error(), "CGO_ENABLED=0 GOOS=linux") {
		t.Errorf("a responder linked with cgo should be refused naming CGO_ENABLED: %v", err)
	}
}
