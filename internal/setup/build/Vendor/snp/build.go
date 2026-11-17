// Package snp is this role's adapter to internal/setup/build/snp, which builds what carries an application into an
// AMD SEV-SNP guest. The work is done once there; this file only converts the input and the result. The Vendor and
// auditor copies are deliberately the same (STRUCTURE.md, "Why role code is copied per role").
package snp

import (
	"context"
	"io"

	core "attestamp/internal/setup/build/snp"
)

// The format each option registers under.
const (
	FormatInitrd = core.FormatInitrd
	FormatVerity = core.FormatVerity
)

// OptionOf returns the option of a format string, and false for a format that is not SEV-SNP.
func OptionOf(format string) (int, bool) {
	switch format {
	case core.FormatInitrd:
		return core.Option1, true
	case core.FormatVerity:
		return core.Option2, true
	}
	return 0, false
}

// Build is the input to one build.
type Build struct {
	AppDir     string // the application tree
	BaseInitrd string // the Vendor's published base initrd (Option 2: built with the verity hook)
	Option     int    // 1: amd-sev-snp-initrd; 2: amd-sev-snp-verity
	OutDir     string // defaults to out
	Verify     bool   // build a second time and fail unless the bytes are the same
}

// Result is what a build produced.
type Result struct {
	Format       string
	Initrd       string
	Image        string // Option 2
	HashTree     string // Option 2
	Record       string // build-value.json, read by launch
	InitrdSHA256 string
	RootHash     string // Option 2
	KernelArgs   string // Option 2: the root hash argument; launch reads it from build-value.json
}

// Run builds; see internal/setup/build/snp.
func (b Build) Run(ctx context.Context, log io.Writer) (Result, error) {
	r, err := core.Build{AppDir: b.AppDir, BaseInitrd: b.BaseInitrd, Option: b.Option, OutDir: b.OutDir, Verify: b.Verify}.Run(ctx, log)
	if err != nil {
		return Result{}, err
	}
	return Result{Format: r.Value.Format, Initrd: r.Initrd, Image: r.Image, HashTree: r.HashTree, Record: r.Record,
		InitrdSHA256: r.Value.InitrdSHA256, RootHash: r.Value.RootHash, KernelArgs: r.Value.KernelArgs}, nil
}
