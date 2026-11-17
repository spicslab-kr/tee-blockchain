// Responder build-info check.
//
// att-ra-responder comes from a separate `go build`. CheckToolchain guards the Go version of the
// att binary itself, and the responder's bytes are what get layered into the enclave image and
// measured (PCR0/PCR2). Those bytes depend on the Go patch version, on -trimpath (which keeps the
// absolute checkout path out of the binary), on -buildvcs (by default Go stamps the git commit,
// its time and whether the tree had uncommitted changes into the binary, and -trimpath does not
// remove them), on CGO_ENABLED (a native Linux build with a C compiler present links the cgo DNS
// resolver and libc, a cross-build does not, and the two files differ), on the Go build ID and on GOOS/GOARCH. Go
// records all of that but the build ID inside every binary it produces (debug/buildinfo), so the file is read and a
// responder that cannot match what the other side builds is refused.
//
// The build ID is a hash of every input of the build, so a change to any package the responder links moved its bytes
// even when no code it can run changed (a9a45f2: one unused function in internal/cli). -ldflags=-buildid= leaves it
// empty. Go does not record -ldflags in the build info of a -trimpath build (they may hold paths), so the build ID is
// read from the binary itself: the linker writes it into the ELF note section .note.go.buildid, and writes no such
// section when it is empty. Measured with go1.26.1 on the responder: with an empty build ID, one unused function added
// to internal/cli, a comment reworded without moving a line and a comment appended after the last function each leave
// the bytes as they are; a comment that adds a line above code still moves them, because the binary keeps the line
// numbers of its code.

package build

import (
	"bytes"
	"debug/buildinfo"
	"debug/elf"
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
	VCS      string // the "vcs" build setting (e.g. "git") when version control was stamped in; empty with -buildvcs=false
	CGO      string // the "CGO_ENABLED" build setting: "0" or "1"; empty if the binary does not record it
	BuildID  string // the Go build ID in the ELF note .note.go.buildid; empty with -ldflags=-buildid= (read for linux binaries only)
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
// error, since nothing says what it is.
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
		case "vcs":
			info.VCS = s.Value
		case "CGO_ENABLED":
			info.CGO = s.Value
		}
	}
	if info.GOOS == "linux" {
		if info.BuildID, err = elfGoBuildID(path); err != nil {
			return info, err
		}
	}
	if info.SHA256, err = SHA256File(path); err != nil {
		return info, err
	}
	return info, nil
}

// elfGoBuildID returns the Go build ID the linker wrote into the ELF note section .note.go.buildid, or "" when the
// binary has no such section (built with -ldflags=-buildid=). The note is the usual ELF layout: name size, descriptor
// size and type (4 bytes each, in the file's byte order), the name "Go" padded to 4 bytes, then the build ID.
func elfGoBuildID(path string) (string, error) {
	f, err := elf.Open(path)
	if err != nil {
		return "", fmt.Errorf("%s: reading it as an ELF file for its Go build ID: %w", path, err)
	}
	defer f.Close()
	s := f.Section(".note.go.buildid")
	if s == nil {
		return "", nil
	}
	d, err := s.Data()
	if err != nil {
		return "", fmt.Errorf("%s: .note.go.buildid: %w", path, err)
	}
	if len(d) < 12 {
		return "", fmt.Errorf("%s: .note.go.buildid is %d bytes, too short for an ELF note", path, len(d))
	}
	nameSize, descSize := f.ByteOrder.Uint32(d[0:]), f.ByteOrder.Uint32(d[4:])
	descStart := 12 + (uint64(nameSize)+3)&^3
	if descStart+uint64(descSize) > uint64(len(d)) {
		return "", fmt.Errorf("%s: .note.go.buildid is malformed", path)
	}
	id := string(bytes.TrimRight(d[descStart:descStart+uint64(descSize)], "\x00"))
	if id == "" {
		// a section with an empty descriptor is not what an empty -buildid gives; say what it holds rather than pass it
		return "(an empty .note.go.buildid section)", nil
	}
	return id, nil
}

// CheckResponder returns an error unless the responder was built with the pinned toolchain, with
// -trimpath, with -buildvcs=false, with CGO_ENABLED=0, for linux, with an empty build ID (-ldflags=-buildid=), and for
// hostGOARCH, the architecture of the parent instance, which is what the enclave runs. Callers pass runtime.GOARCH.
func CheckResponder(info ResponderInfo, hostGOARCH string) error {
	want := RequiredToolchain()
	rebuild := func(goarch string) string {
		return fmt.Sprintf("rebuild it with: GOTOOLCHAIN=%s CGO_ENABLED=0 GOOS=linux GOARCH=%s go build -trimpath -buildvcs=false -ldflags=-buildid= -o att-ra-responder ./cmd/ra-responder", want, goarch)
	}
	if info.Go != want {
		return fmt.Errorf("att-ra-responder was built with %s but the project pins %s (internal/setup/build/GO_TOOLCHAIN); its bytes, and so the measurements, depend on the exact toolchain; %s", info.Go, want, rebuild(hostGOARCH))
	}
	if !info.Trimpath {
		return fmt.Errorf("att-ra-responder was built without -trimpath: the absolute checkout path is baked into it, so the same source built from another folder gives different bytes and different measurements; %s", rebuild(hostGOARCH))
	}
	if info.VCS != "" {
		return fmt.Errorf("att-ra-responder was built with version control stamping: the git commit and commit time are baked into it, so the same source built from a different commit, or from a tree with uncommitted changes, gives different bytes and different measurements; %s", rebuild(hostGOARCH))
	}
	if info.CGO != "0" {
		got := "CGO_ENABLED=" + info.CGO
		if info.CGO == "" {
			got = "no CGO_ENABLED setting"
		}
		return fmt.Errorf("att-ra-responder was built with %s: Go links cgo whenever a C compiler is present and the build is not a cross-build, so the same source built on Linux with gcc and built anywhere else gives different bytes and different measurements; %s", got, rebuild(hostGOARCH))
	}
	if info.GOOS != "linux" {
		return fmt.Errorf("att-ra-responder was built for GOOS=%s but the enclave runs linux; %s", info.GOOS, rebuild(hostGOARCH))
	}
	if info.BuildID != "" {
		return fmt.Errorf("att-ra-responder carries a Go build ID (%s), so it was built without -ldflags=-buildid=: the build ID is a hash of every input of the build, so a change to any package it links gives different bytes and different measurements even when no code it runs changed; %s", info.BuildID, rebuild(hostGOARCH))
	}
	if info.GOARCH != hostGOARCH {
		return fmt.Errorf("att-ra-responder was built for GOARCH=%s but this instance is %s; the enclave runs the parent instance's architecture; %s", info.GOARCH, hostGOARCH, rebuild(hostGOARCH))
	}
	return nil
}
