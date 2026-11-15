// Package tdx is this role's adapter to internal/setup/launch/tdx, which launches a built initrd on an Intel TDX guest
// and reads back what the boot measured. The work is done once there; this file only converts the input and the
// result. The Vendor and auditor copies are deliberately the same (CLAUDE.md, "Role code is deliberately duplicated").
//
// A launch is two runs with a reboot between them: Install, then Check after the guest has booted the new entry.
package tdx

import (
	"context"
	"io"

	core "attestamp/internal/setup/launch/tdx"
)

// Install puts the initrd in /boot and makes a GRUB entry for it the default. It does not reboot.
type Install struct {
	Initrd    string // the built initrd
	ID        string // GRUB entry id; "attestamp" when empty
	ExtraArgs string // appended to the kernel command line (measured into RTMR2)
}

// Installed is what Install wrote.
type Installed struct {
	ID         string
	InitrdPath string
	Entry      string
}

// Run installs; see internal/setup/launch/tdx.
func (in Install) Run(ctx context.Context, log io.Writer) (Installed, error) {
	r, err := core.Install{Initrd: in.Initrd, ID: in.ID, ExtraArgs: in.ExtraArgs}.Run(ctx, log)
	if err != nil {
		return Installed{}, err
	}
	return Installed{ID: r.ID, InitrdPath: r.InitrdPath, Entry: r.Entry}, nil
}

// Check runs after the reboot: it verifies a fresh quote, replays the event log, requires the measured initrd to be
// the built one and the application to have been unpacked, and writes MRTD and RTMR0-2 to OutDir/measurements.json.
type Check struct {
	BuildValue string // the build's build-value.json
	OutDir     string // defaults to out
}

// Measured is what the boot measured.
type Measured struct {
	Values       map[string]string // MRTD, RTMR0, RTMR1, RTMR2
	Measurements string            // the measurements.json written, for register and audit
	InitrdPath   string
	InitrdSHA384 string
	Cmdline      string
}

// Run checks; see internal/setup/launch/tdx.
func (c Check) Run(ctx context.Context, log io.Writer) (Measured, error) {
	r, err := core.Check{BuildValue: c.BuildValue, OutDir: c.OutDir}.Run(ctx, log)
	if err != nil {
		return Measured{}, err
	}
	return Measured{Values: r.Values, Measurements: r.Measurements, InitrdPath: r.InitrdPath, InitrdSHA384: r.InitrdSHA384, Cmdline: r.Cmdline}, nil
}
