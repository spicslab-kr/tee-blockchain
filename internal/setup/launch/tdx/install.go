// Package tdx launches a built initrd on an Intel TDX guest and reads back what the boot measured. It is the
// role-independent core; internal/setup/launch/{Vendor,auditor}/tdx are thin adapters.
//
// On a confidential VM, booting the VM is the launch. It takes two runs with a reboot between them
// (docs/plan/cvm-port.md, "Phase 3 design"):
//
//  1. Install: put the initrd in /boot and write a GRUB entry made from the stock one, as the default
//     (internal/setup/launch/grub, shared with SEV-SNP).
//  2. Check, after the reboot: a fresh quote, verified; the CC event log replayed against its RTMRs; the initrd GRUB
//     loaded must have the build's SHA-384; the boot marker must be there. Then MRTD and RTMR0-2 are written out as the
//     measurements to register (check.go, eventlog.go).
package tdx

import "attestamp/internal/setup/launch/grub"

// Install, Installed, Runner and ExecRunner are the shared GRUB install (internal/setup/launch/grub).
type (
	Install    = grub.Install
	Installed  = grub.Installed
	Runner     = grub.Runner
	ExecRunner = grub.ExecRunner
)

// ErrForcePartuuid: the guest boots without an initrd, so the application could never be loaded.
var ErrForcePartuuid = grub.ErrForcePartuuid

// DefaultID is the GRUB entry id.
const DefaultID = grub.DefaultID
