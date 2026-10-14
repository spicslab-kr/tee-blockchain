// Package auditor is the Auditor's side of the hand-off: it receives build-config.json and sends
// measurement-record.json (with its image file), over a directory or a git repository.
package auditor

import (
	"runtime"
	"runtime/debug"

	"attestamp/internal/setup/build"
)

const SchemaBuildConfig = "attestamp.build-config/1"

// TEEInfoKeys lists the tee_info keys each platform requires.
var TEEInfoKeys = map[string][]string{
	"aws-nitro": {"instance_id", "region", "ami_id"},
	"gcp":       {"instance_name", "project_id", "container_image"},
	"azure":     {"vm_name", "region", "maa_endpoint"},
}

// Source locates the code to build: repository, commit and an optional path inside it.
type Source struct {
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
	Path   string `json:"path,omitempty"`
}

// BuildEnv is the environment the Vendor built in. The Auditor rebuilds in the same one, because the
// enclave image's measurements change with the CPU architecture (kernel, machine code, base image
// layers) and with the Go toolchain that compiled att-ra-responder.
type BuildEnv struct {
	OS       string `json:"os"`       // GOOS, e.g. linux
	Arch     string `json:"arch"`     // GOARCH, e.g. amd64 or arm64
	Go       string `json:"go"`       // runtime.Version() of the att binary, e.g. go1.26.1
	Trimpath bool   `json:"trimpath"` // whether the att binary (and so att-ra-responder) was built with -trimpath
}

// IsZero reports that no field is set (a build-config.json that has no build_env).
func (e BuildEnv) IsZero() bool { return e == BuildEnv{} }

// String formats the environment on one line, e.g. "linux/amd64 go1.26.1 trimpath=true".
func (e BuildEnv) String() string {
	tp := "false"
	if e.Trimpath {
		tp = "true"
	}
	return e.OS + "/" + e.Arch + " " + e.Go + " trimpath=" + tp
}

// CurrentBuildEnv returns the environment this att binary runs in, which its build command uses to
// compile att-ra-responder. Trimpath comes from the binary's own build info.
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

// validArch reports whether arch is one the project builds for (build.ArchTargets). An Auditor can
// rebuild only those.
func validArch(arch string) bool {
	for _, t := range build.ArchTargets {
		if t.GOARCH == arch {
			return true
		}
	}
	return false
}

// archList formats the supported architectures for error messages.
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

// BuildConfig is the build-config.json the Vendor hands to the Auditor: which source to build, for which
// platform, and the TEE info needed to reach the instance.
type BuildConfig struct {
	Schema   string            `json:"schema"`
	Service  string            `json:"service"`
	Version  string            `json:"version"`
	Platform string            `json:"platform"`
	Source   Source            `json:"source"`
	TEEInfo  map[string]string `json:"tee_info"`
	BuildEnv BuildEnv          `json:"build_env"`
}

// Validate lists every problem found, joined with "; ".
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
	// The Auditor rebuilds to build_env, so aws-nitro (measurements differ per architecture and
	// toolchain) requires it. trimpath=false passes validation and the Auditor warns about it.
	if c.Platform == "aws-nitro" {
		if c.BuildEnv.OS == "" {
			errs = append(errs, "build_env.os: required for aws-nitro")
		}
		if c.BuildEnv.Arch == "" {
			errs = append(errs, "build_env.arch: required for aws-nitro")
		}
		if c.BuildEnv.Go == "" {
			errs = append(errs, "build_env.go: required for aws-nitro")
		}
	}
	if c.BuildEnv.Arch != "" && !validArch(c.BuildEnv.Arch) {
		errs = append(errs, "build_env.arch: unknown "+quote(c.BuildEnv.Arch)+" (want one of "+archList()+")")
	}
	return join(errs)
}
