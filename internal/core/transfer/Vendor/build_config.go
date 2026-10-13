// Package vendor transfers files: it sends and receives the build config and the measurement record.
package vendor

import (
	"runtime"
	"runtime/debug"

	"tb/internal/setup/build"
)

const SchemaBuildConfig = "tb.build-config/1"

// TEEInfoKeys lists the required TEE info keys per platform.
var TEEInfoKeys = map[string][]string{
	"aws":   {"instance_id", "region", "ami_id"},
	"gcp":   {"instance_name", "project_id", "container_image"},
	"azure": {"vm_name", "region", "maa_endpoint"},
}

type Source struct {
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
	Path   string `json:"path,omitempty"`
}

// BuildEnv — the environment the Vendor built in. The Auditor must rebuild in the same one:
// the enclave image's measurements change with the CPU architecture (kernel, machine code,
// base image layers) and with the Go toolchain that compiled tb-ra-responder.
type BuildEnv struct {
	OS       string `json:"os"`       // GOOS, e.g. linux
	Arch     string `json:"arch"`     // GOARCH, e.g. amd64 or arm64
	Go       string `json:"go"`       // runtime.Version() of the tb binary, e.g. go1.26.1
	Trimpath bool   `json:"trimpath"` // whether the tb binary (and so tb-ra-responder) was built with -trimpath
}

// IsZero — nothing declared (an older build-config.json, or a hand-written one that left it out).
func (e BuildEnv) IsZero() bool { return e == BuildEnv{} }

// String — one line, e.g. "linux/amd64 go1.26.1 trimpath=true".
func (e BuildEnv) String() string {
	tp := "false"
	if e.Trimpath {
		tp = "true"
	}
	return e.OS + "/" + e.Arch + " " + e.Go + " trimpath=" + tp
}

// CurrentBuildEnv — the environment this tb binary is running in, which is what its build
// command uses to compile tb-ra-responder. -trimpath is read from the binary's own build info.
func CurrentBuildEnv() BuildEnv {
	e := BuildEnv{OS: runtime.GOOS, Arch: runtime.GOARCH, Go: runtime.Version()}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "-trimpath" && s.Value == "true" {
				e.Trimpath = true
			}
		}
	}
	return e
}

// validArch — arch must be one the project builds for (build.ArchTargets); anything else
// can't be rebuilt by an Auditor, so it is refused before it ever reaches one.
func validArch(arch string) bool {
	for _, t := range build.ArchTargets {
		if t.GOARCH == arch {
			return true
		}
	}
	return false
}

func archList() string {
	s := ""
	for i, t := range build.ArchTargets {
		if i > 0 {
			s += ", "
		}
		s += t.GOARCH
	}
	return s
}

type BuildConfig struct {
	Schema   string            `json:"schema"`
	Service  string            `json:"service"`
	Version  string            `json:"version"`
	Platform string            `json:"platform"`
	Source   Source            `json:"source"`
	TEEInfo  map[string]string `json:"tee_info"`
	BuildEnv BuildEnv          `json:"build_env"`
}

func (c BuildConfig) Validate() error {
	var errs []string
	if c.Schema != SchemaBuildConfig {
		errs = append(errs, "schema: want "+SchemaBuildConfig)
	}
	if c.Service == "" {
		errs = append(errs, "service: empty")
	}
	if c.Version == "" {
		errs = append(errs, "version: empty")
	}
	keys, ok := TEEInfoKeys[c.Platform]
	if !ok {
		errs = append(errs, "platform: unknown "+quote(c.Platform))
	}
	if c.Source.Repo == "" || c.Source.Commit == "" {
		errs = append(errs, "source: repo and commit required")
	}
	for _, k := range keys {
		if c.TEEInfo[k] == "" {
			errs = append(errs, "tee_info."+k+": required for "+c.Platform)
		}
	}
	// build_env: the Auditor rebuilds to it, so on aws (where the measurements are per
	// architecture and per toolchain) it must be declared. trimpath=false is allowed here; the
	// Auditor warns on it instead.
	if c.Platform == "aws" {
		if c.BuildEnv.OS == "" {
			errs = append(errs, "build_env.os: required for aws")
		}
		if c.BuildEnv.Arch == "" {
			errs = append(errs, "build_env.arch: required for aws")
		}
		if c.BuildEnv.Go == "" {
			errs = append(errs, "build_env.go: required for aws")
		}
	}
	if c.BuildEnv.Arch != "" && !validArch(c.BuildEnv.Arch) {
		errs = append(errs, "build_env.arch: unknown "+quote(c.BuildEnv.Arch)+" (want one of "+archList()+")")
	}
	return join(errs)
}
