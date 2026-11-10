// Toolchain pin.
//
// att-ra-responder is layered straight into the enclave image, so the image's measurements
// (PCR0/PCR2) depend on the exact bytes of that binary, and those bytes change with the Go
// patch version (measured: go1.26.2 and go1.26.8 give different binaries even with -trimpath).
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
// rejected too, since its bytes are not the pinned toolchain's bytes.
func CheckToolchain() error {
	return checkToolchain(runtime.Version(), RequiredToolchain())
}

func checkToolchain(have, want string) error {
	if have == want {
		return nil
	}
	return fmt.Errorf("att was built with %s but the project pins %s (internal/setup/build/GO_TOOLCHAIN); the responder's bytes, and so the measurements, depend on the exact toolchain; rebuild att with %s (GOTOOLCHAIN=%s go build ...)", have, want, want, want)
}
