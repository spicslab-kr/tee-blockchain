package build

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// checkoutForArchTest makes an independent copy of the module (cmd/, internal/, go.mod, go.sum)
// at a uniquely-named temp directory, via `git archive` of the current commit. Two checkouts of
// the same commit this way never share a path — exactly the scenario where the checkout path leaked into the binary
// (Vendor and Auditor cloning the same source into different directories).
func checkoutForArchTest(t *testing.T, name string) string {
	t.Helper()
	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := exec.Command("git", "archive", "HEAD", "cmd", "internal", "go.mod", "go.sum")
	archive.Dir = repoRoot
	tarOut, err := archive.Output()
	if err != nil {
		t.Fatalf("git archive: %v", err)
	}
	untar := exec.Command("tar", "-x")
	untar.Dir = dir
	untar.Stdin = bytes.NewReader(tarOut)
	if out, err := untar.CombinedOutput(); err != nil {
		t.Fatalf("tar -x: %v\n%s", err, out)
	}
	return dir
}

// TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths is the checkout-path regression check: the
// same source, same architecture, built from two differently-named checkout directories, must
// produce byte-identical binaries. If this fails, -trimpath has regressed (or been dropped)
// from BuildResponderForArch.
func TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles a real binary; slow")
	}
	a := checkoutForArchTest(t, "checkout-a")
	b := checkoutForArchTest(t, "checkout-b-a-different-name-on-purpose")
	target := ArchTargets[0]
	outA := filepath.Join(t.TempDir(), "responder-a")
	outB := filepath.Join(t.TempDir(), "responder-b")
	if err := BuildResponderForArch(context.Background(), a, target, outA); err != nil {
		t.Fatal(err)
	}
	if err := BuildResponderForArch(context.Background(), b, target, outB); err != nil {
		t.Fatal(err)
	}
	shaA, err := SHA256File(outA)
	if err != nil {
		t.Fatal(err)
	}
	shaB, err := SHA256File(outB)
	if err != nil {
		t.Fatal(err)
	}
	if shaA != shaB {
		t.Fatalf("same source (%s), different checkout paths, different hash: %s vs %s — -trimpath regression",
			target.Name, shaA, shaB)
	}
}

// TestBuildResponderAllArches_DiffersAcrossArchitectures documents and checks the per-architecture behaviour:
// building the same source for two different architectures must NOT collide on the same hash —
// each architecture's machine code is genuinely different, so a collision would mean the build
// silently produced the wrong architecture's binary (or GOARCH was ignored).
func TestBuildResponderAllArches_DiffersAcrossArchitectures(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles a real binary per architecture; slow")
	}
	if len(ArchTargets) < 2 {
		t.Skip("need at least two targets to compare")
	}
	repoRoot := checkoutForArchTest(t, "checkout")
	reports, err := BuildResponderAllArches(context.Background(), repoRoot, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, r := range reports {
		if prev, ok := seen[r.SHA256]; ok {
			t.Errorf("%s and %s produced the same responder hash %s — expected different machine code per architecture",
				prev, r.Target.Name, r.SHA256)
		}
		seen[r.SHA256] = r.Target.Name
	}
}
