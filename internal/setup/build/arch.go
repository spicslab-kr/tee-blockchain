// Package build holds tooling shared by the Vendor and Auditor build packages (aws.Build lives
// under Vendor/aws and auditor/aws; this package is the role-agnostic part).
//
// Arch cross-builds att-ra-responder for every CPU architecture this project ships to, so the
// per-architecture hash can be produced and compared the same way everywhere.
//
// A different architecture is expected to produce a different att-ra-responder binary, and with
// it different PCR0/PCR2 once it's layered into an enclave image — Nitro's kernel, the compiled
// machine code, and the base image layers all differ per architecture. That is expected. What must stay equal is the hash for one architecture built from two different checkout
// paths, which is why every build here passes -trimpath.
//
// Add an architecture by adding one line to ArchTargets; nothing else in this package changes.
package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ArchTarget is one CPU architecture this project builds att-ra-responder for.
type ArchTarget struct {
	Name   string // human label, e.g. "x86_64" — used in reports and instance-spec docs
	GOARCH string // Go's GOARCH value for cross-compiling
}

// ArchTargets — every architecture the project currently ships to. Extend this list to add
// coverage for another architecture; BuildResponderAllArches and the reproducibility tests pick it up
// automatically.
var ArchTargets = []ArchTarget{
	{Name: "x86_64", GOARCH: "amd64"},
	{Name: "arm64", GOARCH: "arm64"},
}

// BuildResponderForArch cross-compiles cmd/ra-responder from repoRoot (a directory containing
// go.mod) for target.GOARCH, with -trimpath, which keeps the checkout path out of the
// binary. The responder only ever runs inside a Nitro enclave, so GOOS is always linux.
func BuildResponderForArch(ctx context.Context, repoRoot string, target ArchTarget, outPath string) error {
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", outPath, "./cmd/ra-responder")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+target.GOARCH)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build (GOARCH=%s) in %s: %w\n%s", target.GOARCH, repoRoot, err, out)
	}
	return nil
}

// SHA256File returns the lowercase hex sha256 of the file at path.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ArchReport is one row of a per-architecture comparison: which architecture, and the sha256 of
// the att-ra-responder binary BuildResponderAllArches produced for it.
type ArchReport struct {
	Target ArchTarget
	Path   string
	SHA256 string
}

// BuildResponderAllArches cross-builds att-ra-responder for every entry in ArchTargets, writing
// each binary to dir/att-ra-responder-<GOARCH> and hashing it. The returned reports are in
// ArchTargets order.
func BuildResponderAllArches(ctx context.Context, repoRoot, dir string) ([]ArchReport, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	reports := make([]ArchReport, 0, len(ArchTargets))
	for _, t := range ArchTargets {
		out := dir + "/att-ra-responder-" + t.GOARCH
		if err := BuildResponderForArch(ctx, repoRoot, t, out); err != nil {
			return nil, err
		}
		sum, err := SHA256File(out)
		if err != nil {
			return nil, err
		}
		reports = append(reports, ArchReport{Target: t, Path: out, SHA256: sum})
	}
	return reports, nil
}
