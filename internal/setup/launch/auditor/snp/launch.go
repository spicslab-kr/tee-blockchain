// Package snp is this role's adapter to internal/setup/launch/snp, which launches a SEV-SNP build on a GCP guest and
// reads back what the boot measured. The work is done once there; this file only converts the input and the result.
// The Vendor and auditor copies are deliberately the same (STRUCTURE.md, "Why role code is copied per role").
//
// A launch is two runs with a reboot between them: Install, then Check after the guest has booted the new entry.
package snp

import (
	"context"
	"io"

	core "attestamp/internal/setup/launch/snp"
)

// Install installs a build: the files next to its build-value.json go to the guest. It does not reboot.
type Install struct {
	BuildValue string // the build's build-value.json
	ID         string // GRUB entry id; "attestamp" when empty
}

// Installed is what Install wrote.
type Installed struct {
	ID, InitrdPath, Entry string
}

// Run installs; see internal/setup/launch/snp.
func (in Install) Run(ctx context.Context, log io.Writer) (Installed, error) {
	r, err := core.Install{BuildValue: in.BuildValue, ID: in.ID}.Run(ctx, log)
	if err != nil {
		return Installed{}, err
	}
	return Installed{ID: r.ID, InitrdPath: r.InitrdPath, Entry: r.Entry}, nil
}

// Check runs after the reboot: it verifies fresh evidence, requires INITRD (and for Option 2 PCR12) to be the built
// values and the application to have been unpacked or mounted, and writes the values to OutDir/measurements.json.
type Check struct {
	BuildValue string
	OutDir     string // defaults to out
}

// Measured is what the boot measured.
type Measured struct {
	Format       string
	Values       map[string]string
	Measurements string
}

// Run checks; see internal/setup/launch/snp.
func (c Check) Run(ctx context.Context, log io.Writer) (Measured, error) {
	r, err := core.Check{BuildValue: c.BuildValue, OutDir: c.OutDir}.Run(ctx, log)
	if err != nil {
		return Measured{}, err
	}
	return Measured{Format: r.Format, Values: r.Values, Measurements: r.Measurements}, nil
}
