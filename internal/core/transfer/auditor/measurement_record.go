package auditor

import (
	"fmt"
	"strings"

	"attestamp/internal/setup/build"
	p "attestamp/internal/tee/parse/auditor"
)

const SchemaMeasurementRecord = "attestamp.measurement-record/1"

// MeasurementNames lists the required measurement names per platform.
var MeasurementNames = map[string][]string{
	"aws-nitro": {"PCR0", "PCR1", "PCR2"},
	"gcp":       {"image_digest"},
	"azure":     {}, // digest (container) or image_id (VM); see Image
}

// Image identifies the measured image: aws-nitro uses a file plus sha256, gcp a digest, azure a digest or an image_id.
type Image struct {
	Path    string `json:"path,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Digest  string `json:"digest,omitempty"` // "sha256:<hex>"
	ImageID string `json:"image_id,omitempty"`
}

// BuildTool names the tool that built the image and its version.
type BuildTool struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Registration names the chain and registry address where the measurement was registered. It is optional.
type Registration struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
}

// MeasurementRecord is the measurement-record.json the Auditor hands to the Vendor: the image, its
// measurements and the build environment they came from.
type MeasurementRecord struct {
	Schema       string         `json:"schema"`
	Service      string         `json:"service"`
	Version      string         `json:"version"`
	Platform     string         `json:"platform"`
	BuildEnv     BuildEnv       `json:"build_env"` // the environment the image was built and measured in, in the shape of build-config.json's build_env. Measurements differ per architecture and Go toolchain, so each service/version has one record per architecture.
	Image        Image          `json:"image"`
	Values       p.Measurements `json:"values"`
	BuildTool    BuildTool      `json:"build_tool"`
	Registration *Registration  `json:"registration,omitempty"`
}

// Validate lists every problem found, joined with "; ".
func (r MeasurementRecord) Validate() error {
	var errs []string
	if r.Schema != SchemaMeasurementRecord {
		errs = append(errs, "schema: want "+SchemaMeasurementRecord)
	}
	if r.Service == "" {
		errs = append(errs, "service: empty")
	}
	if r.Version == "" {
		errs = append(errs, "version: empty")
	}
	names, ok := MeasurementNames[r.Platform]
	if !ok {
		errs = append(errs, "platform: unknown "+quote(r.Platform))
	}
	switch r.Platform {
	case "aws-nitro":
		// The Vendor reads build_env to learn the architecture and Go toolchain of the measured image,
		// so aws-nitro requires all of it.
		if r.BuildEnv.OS == "" {
			errs = append(errs, "build_env.os: required for aws-nitro")
		}
		if !validArch(r.BuildEnv.Arch) {
			errs = append(errs, "build_env.arch: want one of "+strings.Join(archNames(), "|")+" (GOARCH spelling), got "+quote(r.BuildEnv.Arch))
		}
		if r.BuildEnv.Go == "" {
			errs = append(errs, "build_env.go: required for aws-nitro")
		}
		if r.Image.Path == "" {
			errs = append(errs, "image.path: EIF file required for aws-nitro")
		}
		if !isHex(r.Image.SHA256, 64) {
			errs = append(errs, "image.sha256: want 64 hex chars")
		}
		if r.BuildTool.Version == "" {
			errs = append(errs, "build_tool.version: required for aws-nitro")
		}
	case "gcp":
		if !isDigest(r.Image.Digest) {
			errs = append(errs, "image.digest: want sha256:<64 hex> (an image digest)")
		}
	case "azure":
		if !isDigest(r.Image.Digest) && r.Image.ImageID == "" {
			errs = append(errs, "image: digest (container) or image_id (VM) required for azure")
		}
	}
	// As in build-config, every platform accepts only architectures the project builds for.
	if r.Platform != "aws-nitro" && r.BuildEnv.Arch != "" && !validArch(r.BuildEnv.Arch) {
		errs = append(errs, "build_env.arch: unknown "+quote(r.BuildEnv.Arch)+" (want one of "+archList()+")")
	}
	if len(r.Values) == 0 {
		errs = append(errs, "values: empty")
	}
	have := map[string]bool{}
	for i, m := range r.Values {
		if m.Name == "" || len(m.Value) == 0 {
			errs = append(errs, fmt.Sprintf("values[%d]: name and value required", i))
		}
		have[m.Name] = true
	}
	for _, n := range names {
		if !have[n] {
			errs = append(errs, "values: missing "+n+" for "+r.Platform)
		}
	}
	if r.BuildTool.Name == "" {
		errs = append(errs, "build_tool.name: empty")
	}
	if r.Registration != nil && (r.Registration.Chain == "" || r.Registration.Address == "") {
		errs = append(errs, "registration: chain and address required when present")
	}
	return join(errs)
}

// archNames returns the GOARCH names listed in build.ArchTargets.
func archNames() []string {
	out := make([]string, 0, len(build.ArchTargets))
	for _, t := range build.ArchTargets {
		out = append(out, t.GOARCH)
	}
	return out
}
