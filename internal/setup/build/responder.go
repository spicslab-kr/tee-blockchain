// Responder build-info check.
//
// att-ra-responder comes from a separate `go build`. CheckToolchain guards the Go version of the
// att binary itself, and the responder's bytes are what get layered into the enclave image and
// measured (PCR0/PCR2). Those bytes depend on the Go patch version, on -trimpath (which keeps the
// absolute checkout path out of the binary) and on GOOS/GOARCH. Go records all of that inside every
// binary it produces (debug/buildinfo), so the file is read and a responder that cannot match
// what the other side builds is refused.

package build

import (
	"debug/buildinfo"
	"fmt"
	"os"
)

// ResponderInfo is what the att-ra-responder file next to att was built with, read from the Go
// build info the compiler leaves inside every binary.
type ResponderInfo struct {
	Go       string // GoVersion, e.g. "go1.26.1"
	GOOS     string
	GOARCH   string
	Trimpath bool   // -trimpath was given to go build
	SHA256   string // lowercase hex sha256 of the whole file
}

// String is one line for the build log: name, hash prefix, and the build facts, so Vendor and
// Auditor can compare their responders by eye.
func (i ResponderInfo) String() string {
	trim := "-trimpath"
	if !i.Trimpath {
		trim = "(no -trimpath)"
	}
	sum := i.SHA256
	if len(sum) > 16 {
		sum = sum[:16]
	}
	return fmt.Sprintf("responder: att-ra-responder sha256 %s · %s %s/%s %s", sum, i.Go, i.GOOS, i.GOARCH, trim)
}

// ReadResponder reads the Go build info embedded in the binary at path and hashes the file.
// A file with no Go build info (not a Go binary, or one whose build info was stripped) is an
// error — there is no way to tell what it is.
func ReadResponder(path string) (ResponderInfo, error) {
	var info ResponderInfo
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return info, statErr
		}
		return info, fmt.Errorf("%s has no Go build info; is it really att-ra-responder built from ./cmd/ra-responder? (%v)", path, err)
	}
	info.Go = bi.GoVersion
	for _, s := range bi.Settings {
		switch s.Key {
		case "GOOS":
			info.GOOS = s.Value
		case "GOARCH":
			info.GOARCH = s.Value
		case "-trimpath":
			info.Trimpath = s.Value == "true"
		}
	}
	if info.SHA256, err = SHA256File(path); err != nil {
		return info, err
	}
	return info, nil
}

// CheckResponder returns an error unless the responder was built with the pinned toolchain, with
// -trimpath, for linux, and for hostGOARCH — the architecture of the parent instance, which is
// what the enclave runs. Callers pass runtime.GOARCH.
func CheckResponder(info ResponderInfo, hostGOARCH string) error {
	want := RequiredToolchain()
	rebuild := func(goarch string) string {
		return fmt.Sprintf("rebuild it with: GOTOOLCHAIN=%s GOOS=linux GOARCH=%s go build -trimpath -o att-ra-responder ./cmd/ra-responder", want, goarch)
	}
	if info.Go != want {
		return fmt.Errorf("att-ra-responder was built with %s but the project pins %s (internal/setup/build/GO_TOOLCHAIN) — its bytes, and so the measurements, depend on the exact toolchain; %s", info.Go, want, rebuild(hostGOARCH))
	}
	if !info.Trimpath {
		return fmt.Errorf("att-ra-responder was built without -trimpath — the absolute checkout path is baked into it, so the same source built from another folder gives different bytes and different measurements; %s", rebuild(hostGOARCH))
	}
	if info.GOOS != "linux" {
		return fmt.Errorf("att-ra-responder was built for GOOS=%s but the enclave runs linux; %s", info.GOOS, rebuild(hostGOARCH))
	}
	if info.GOARCH != hostGOARCH {
		return fmt.Errorf("att-ra-responder was built for GOARCH=%s but this instance is %s — the enclave runs the parent instance's architecture; %s", info.GOARCH, hostGOARCH, rebuild(hostGOARCH))
	}
	return nil
}
