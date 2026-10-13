package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCheckResponder_Rules — the four rules on hand-made values, so `-short` still covers them
// without compiling anything. Each refusal must name the actual and the expected value and say
// how to rebuild.
func TestCheckResponder_Rules(t *testing.T) {
	want := RequiredToolchain()
	good := ResponderInfo{Go: want, GOOS: "linux", GOARCH: "amd64", Trimpath: true, SHA256: strings.Repeat("ab", 32)}
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
			[]string{"without -trimpath", "go build -trimpath"}},
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

// buildResponderNoTrimpath — like BuildResponderForArch but deliberately without -trimpath, to
// produce the artifact the check must refuse.
func buildResponderNoTrimpath(t *testing.T, repoRoot, goarch, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, "./cmd/ra-responder")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+goarch)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build without -trimpath: %v\n%s", err, o)
	}
}

// TestReadResponder_RealBuilds cross-builds real responders and runs the check on the actual
// artifacts: the correct one passes, the same file is refused for another host architecture,
// and one built without -trimpath is refused naming -trimpath.
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
	if info.GOOS != "linux" || info.GOARCH != "amd64" || !info.Trimpath {
		t.Errorf("unexpected build facts: %+v", info)
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
}
