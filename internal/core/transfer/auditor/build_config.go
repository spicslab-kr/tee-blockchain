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
	"intel-tdx": {"instance_name", "project_id", "zone", "image"},
	// SEV-SNP runs on GCP like TDX, so it needs the same keys to reach the instance.
	"amd-sev-snp-initrd": {"instance_name", "project_id", "zone", "image"},
	"amd-sev-snp-verity": {"instance_name", "project_id", "zone", "image"},
	"gcp":                {"instance_name", "project_id", "container_image"},
	"azure":              {"vm_name", "region", "maa_endpoint"},
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

	// aws-nitro only, and required there; written by att-vendor send on the Vendor's parent instance after its build.
	// build installs docker and aws-nitro-enclaves-cli with dnf and no version, and both decide the measurements.
	// AMIRelease is the Amazon Linux release (e.g. 2023.12.20261109), which the Auditor's receive compares before
	// anything is installed: a proxy for the versions, since each release has its own package repository, and no
	// guarantee (a dnf update with another release, releasever=latest or a package installed by hand moves the
	// versions without it). Docker and NitroCLI are the package versions themselves, which the Auditor's build
	// compares right after installing its own (internal/setup/build, NitroTools).
	AMIRelease string `json:"ami_release,omitempty"`
	Docker     string `json:"docker,omitempty"`    // rpm version-release, e.g. 25.0.18-1.amzn2023.0.3
	NitroCLI   string `json:"nitro_cli,omitempty"` // rpm version-release of aws-nitro-enclaves-cli, e.g. 1.5.0-0.amzn2023
}

// IsZero reports that no field is set (a build-config.json that has no build_env).
func (e BuildEnv) IsZero() bool { return e == BuildEnv{} }

// String formats the environment on one line, e.g. "linux/amd64 go1.26.1 trimpath=true", followed by the Nitro
// fields that are set, e.g. " ami_release=2023.12.20261109 docker=25.0.18-1.amzn2023.0.3 nitro_cli=1.5.0-0.amzn2023".
func (e BuildEnv) String() string {
	tp := "false"
	if e.Trimpath {
		tp = "true"
	}
	s := e.OS + "/" + e.Arch + " " + e.Go + " trimpath=" + tp
	for _, f := range []struct{ k, v string }{{"ami_release", e.AMIRelease}, {"docker", e.Docker}, {"nitro_cli", e.NitroCLI}} {
		if f.v != "" {
			s += " " + f.k + "=" + f.v
		}
	}
	return s
}

// CurrentBuildEnv returns the environment this att binary runs in: send records it as build_env and receive compares
// it with the Vendor's. Trimpath comes from the binary's own build info.
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
	// The Auditor rebuilds to build_env, so every platform with a build requires it (needsBuildEnv: aws-nitro,
	// intel-tdx and both amd-sev-snp formats). trimpath=false passes validation and the Auditor warns about it.
	if needsBuildEnv(c.Platform) {
		if c.BuildEnv.OS == "" {
			errs = append(errs, "build_env.os: required for "+c.Platform)
		}
		if c.BuildEnv.Arch == "" {
			errs = append(errs, "build_env.arch: required for "+c.Platform)
		}
		if c.BuildEnv.Go == "" {
			errs = append(errs, "build_env.go: required for "+c.Platform)
		}
	}
	// On aws-nitro the docker and nitro-cli build installs decide the measurements too. Required since 2026-11-17, so
	// that no config can skip the Auditor's two checks (receive: ami_release; build --config: docker, nitro_cli).
	// att-vendor send fills them in on the instance that ran build.
	if c.Platform == "aws-nitro" {
		for _, f := range []struct{ k, v string }{{"ami_release", c.BuildEnv.AMIRelease}, {"docker", c.BuildEnv.Docker}, {"nitro_cli", c.BuildEnv.NitroCLI}} {
			if f.v == "" {
				errs = append(errs, "build_env."+f.k+": required for aws-nitro (att-vendor send fills it in on the instance that ran build)")
			}
		}
	}
	if c.BuildEnv.Arch != "" && !validArch(c.BuildEnv.Arch) {
		errs = append(errs, "build_env.arch: unknown "+quote(c.BuildEnv.Arch)+" (want one of "+archList()+")")
	}
	return join(errs)
}

// needsBuildEnv reports whether a platform's measurements depend on the build environment. On aws-nitro the EIF
// carries att-ra-responder, whose bytes change with the CPU architecture and the Go toolchain. On intel-tdx and
// amd-sev-snp-* the responder is not measured, but att itself writes the measured initrd or disk image, and an
// application compiled for it depends on the architecture and toolchain as well.
func needsBuildEnv(platform string) bool {
	switch platform {
	case "aws-nitro", "intel-tdx", "amd-sev-snp-initrd", "amd-sev-snp-verity":
		return true
	}
	return false
}
