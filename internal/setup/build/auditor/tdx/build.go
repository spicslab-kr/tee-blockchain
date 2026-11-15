// Package tdx is this role's adapter to internal/setup/build/tdx, which builds the initrd that carries an application
// into an Intel TDX guest. The work is done once there; this file only converts the input and the result, as
// internal/tee/parse/<role>/aws.go does for the Nitro parser. The Vendor and auditor copies are deliberately the same
// (CLAUDE.md, "Role code is deliberately duplicated").
package tdx

import (
	"context"
	"io"

	core "attestamp/internal/setup/build/tdx"
)

// Format is the format string the built guest attests under, for either option.
const Format = core.Format

// Build is the input to one build.
type Build struct {
	AppDir     string // the application tree, put under /opt/app
	BaseInitrd string // the Vendor's published base initrd
	Option     int    // 1: the tree as an archive in the initrd; 2: a squashfs + dm-verity image in the initrd
	OutDir     string // defaults to out
	Verify     bool   // build a second time and fail unless the initrd is the same
}

// Result is what a build produced.
type Result struct {
	Initrd       string // the combined initrd
	Record       string // build-value.json, read by launch
	InitrdSHA384 string // the digest GRUB extends into RTMR2
	RootHash     string // Option 2 only
	Option       int
}

// Run builds; see internal/setup/build/tdx.
func (b Build) Run(ctx context.Context, log io.Writer) (Result, error) {
	r, err := core.Build{AppDir: b.AppDir, BaseInitrd: b.BaseInitrd, Option: b.Option, OutDir: b.OutDir, Verify: b.Verify}.Run(ctx, log)
	if err != nil {
		return Result{}, err
	}
	return Result{Initrd: r.Initrd, Record: r.Record, InitrdSHA384: r.Value.InitrdSHA384, RootHash: r.Value.RootHash, Option: r.Value.Option}, nil
}
