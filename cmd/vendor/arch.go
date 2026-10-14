package main

import (
	"fmt"
	"runtime"
	"strings"

	"attestamp/internal/cli"
	tr "attestamp/internal/core/transfer/Vendor"
	"attestamp/internal/setup/build"
)

// archNames: the GOARCH names of the architectures this project builds (amd64, arm64). build.ArchTargets is the only source.
func archNames() []string {
	out := make([]string, 0, len(build.ArchTargets))
	for _, t := range build.ArchTargets {
		out = append(out, t.GOARCH)
	}
	return out
}

// defaultArch is used when --arch is not given: the GOARCH of the machine this binary runs on. If the measurements file
// came from another machine (instance) and is registered here, pass that machine's architecture with --arch: the same source has different PCRs per architecture.
func defaultArch() string { return runtime.GOARCH }

// checkArch accepts only GOARCH spellings. The x86_64 and aarch64 that uname prints are rejected here.
func checkArch(s string) error {
	for _, n := range archNames() {
		if s == n {
			return nil
		}
	}
	return fmt.Errorf("--arch: want one of %s (GOARCH spelling: x86_64 is amd64, aarch64 is arm64), got %q", strings.Join(archNames(), "|"), s)
}

// warnToolchain prints one line if the record's build_env.go differs from the Go this project pins (internal/setup/build/GO_TOOLCHAIN).
// It is a warning only, since the record may come from another checkout; a rebuild here would produce different bytes.
func warnToolchain(env tr.BuildEnv) {
	if env.Go != "" && env.Go != build.RequiredToolchain() {
		cli.Errorf("warning: the record was produced with %s, but this project pins %s — a rebuild here will not reproduce its measurements", env.Go, build.RequiredToolchain())
	}
}
