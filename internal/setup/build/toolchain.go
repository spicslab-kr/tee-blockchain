// Toolchain pin.
//
// On AWS Nitro, att-ra-responder is layered straight into the enclave image, so the image's
// measurements (PCR0/PCR2) depend on the exact bytes of that binary, and those bytes change with
// the Go patch version (measured: go1.26.2 and go1.26.8 give different binaries even with
// -trimpath). On Intel TDX and AMD SEV-SNP the responder is not measured; the measured initrd (and
// Option 2's image) is written by att itself, so the pin keeps both sides on the same att.
// The Auditor therefore receives the toolchain version together with the source, and every att
// binary refuses to build under any other. GO_TOOLCHAIN in this directory is that pin: the
// single source of truth, embedded here, checked at the start of every build and read by the
// e2e setup scripts. A toolchain line in go.mod cannot hold the pin because `go mod tidy` strips
// it even when it matches the go directive. The file lives inside this package directory because
// //go:embed cannot reach a parent directory.

package build

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
)

//go:embed GO_TOOLCHAIN
var requiredToolchain string

// RequiredToolchain returns the pinned Go toolchain, e.g. "go1.26.1": the contents of
// GO_TOOLCHAIN with surrounding whitespace removed.
func RequiredToolchain() string {
	return strings.TrimSpace(requiredToolchain)
}

// ToolchainLine is the one-line description every att binary prints (build log, `version`):
// the pin, where it lives, and what this binary was actually built with.
func ToolchainLine() string {
	return fmt.Sprintf("toolchain: %s (pinned: internal/setup/build/GO_TOOLCHAIN) · this binary: %s", RequiredToolchain(), runtime.Version())
}

// CheckToolchain returns an error unless this att binary was built with exactly the pinned
// toolchain. The comparison is exact-string: runtime.Version() of a development toolchain
// carries a suffix ("go1.26.1-devel_…" or "devel go1.27-…"), and such a build is deliberately
// rejected too, since its bytes are not the pinned toolchain's bytes. With no platform known
// (`version`), the message gives the reason for every platform; a build uses CheckToolchainFor.
func CheckToolchain() error {
	return checkToolchainFor(runtime.Version(), RequiredToolchain(), "")
}

// CheckToolchainFor is CheckToolchain for a build of one platform (a format string): the refusal is
// the same everywhere, but what the pin protects is not. On AWS Nitro att-ra-responder is inside
// the measured image. On Intel TDX and AMD SEV-SNP it runs from the guest's disk and is not
// measured (measured 2026-11-16: a Go application built with go1.26.1 and go1.26.2 gave two RTMR2
// values, while the responder's version never reaches one); what is measured is the initrd, and
// for Option 2 the image, that att itself writes, so the pin keeps the Vendor's and the Auditor's
// att the same tool.
func CheckToolchainFor(platform string) error {
	return checkToolchainFor(runtime.Version(), RequiredToolchain(), platform)
}

func checkToolchainFor(have, want, platform string) error {
	if have == want {
		return nil
	}
	return fmt.Errorf("att was built with %s but the project pins %s (internal/setup/build/GO_TOOLCHAIN); %s; rebuild att with %s (GOTOOLCHAIN=%s go build ...)", have, want, pinReason(platform), want, want)
}

// pinReason says what a toolchain mismatch would change on that platform.
func pinReason(platform string) string {
	switch {
	case platform == "aws-nitro":
		return "on aws-nitro att-ra-responder is layered into the enclave image, so its bytes, and so PCR0 and PCR2, depend on the exact toolchain"
	case platform == "intel-tdx" || strings.HasPrefix(platform, "amd-sev-snp"):
		return "on " + platform + " att-ra-responder is not measured; the measurement covers the initrd (and for Option 2 the image) that att itself writes, so the Vendor and the Auditor must run the same att"
	default:
		return "on aws-nitro att-ra-responder is inside the measured image; on intel-tdx and amd-sev-snp it is not, and the measured initrd is written by att itself, so the Vendor and the Auditor must run the same att"
	}
}
