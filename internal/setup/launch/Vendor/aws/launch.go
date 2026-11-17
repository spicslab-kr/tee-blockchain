// Package aws is this role's adapter to internal/setup/launch/aws, which launches an EIF as a Nitro enclave, bridges
// TCP to its vsock port and reports (and only on request opens) the RA port on the security group. The work is done
// once there; this file only converts the input and the result. The Vendor and auditor copies are deliberately the
// same (STRUCTURE.md, "Why role code is copied per role").
package aws

import (
	"context"
	"io"

	core "attestamp/internal/setup/launch/aws"
)

// Launch is the input to one launch; see internal/setup/launch/aws for each field.
type Launch struct {
	EnclaveName string // enclave name. Defaults to the EIF file name
	EIF         string // enclave image file
	Port        int    // RA port. 0 means 5000
	Memory      int    // enclave memory in MiB. 0 means sized from the EIF size
	CPUs        int    // 0 means 2
	CID         int    // enclave vsock address. 0 means 16
	Listen      string // address the bridge accepts on. Defaults to 0.0.0.0
	ProxyBinary string // att binary that runs ra-proxy. Defaults to the running executable
	OutDir      string // bridge log directory. Defaults to out

	// BuildMeasurements is the measurements.json that came with an EIF received from someone else; the EIF is not
	// launched if measuring it gives another value.
	BuildMeasurements string
}

// Result is what goes into the configuration.
type Result struct {
	Name         string
	Measurements string            // out/measurements.json
	PCR          map[string]string // PCR0 · PCR1 · PCR2
	Memory       int               // memory actually given, in MiB
	Address      string            // address to check from this instance: 127.0.0.1:port
}

// Run launches; see internal/setup/launch/aws.
func (l Launch) Run(ctx context.Context, log io.Writer) (Result, error) {
	r, err := core.Launch{
		EnclaveName: l.EnclaveName, EIF: l.EIF, Port: l.Port, Memory: l.Memory, CPUs: l.CPUs, CID: l.CID,
		Listen: l.Listen, ProxyBinary: l.ProxyBinary, OutDir: l.OutDir, BuildMeasurements: l.BuildMeasurements,
	}.Run(ctx, log)
	if err != nil {
		return Result{}, err
	}
	return Result{Name: r.Name, Measurements: r.Measurements, PCR: r.PCR, Memory: r.Memory, Address: r.Address}, nil
}

// ParseCIDR checks an IPv4 CIDR for --open-port-to; see internal/setup/launch/aws.
func ParseCIDR(s string) (string, error) { return core.ParseCIDR(s) }

// ReportPort reports the RA port on the security group, and opens it to cidr only when cidr is given; see
// internal/setup/launch/aws.
func ReportPort(ctx context.Context, port int, cidr string, log io.Writer) {
	core.ReportPort(ctx, port, cidr, log)
}
