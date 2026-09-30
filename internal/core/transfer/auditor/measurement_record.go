package auditor

import (
	"fmt"
	"strings"

	"tb/internal/setup/build"
	p "tb/internal/tee/parse/auditor"
)

const SchemaMeasurementRecord = "tb.measurement-record/1"

// MeasurementNames — 플랫폼별 필수 측정값 이름.
var MeasurementNames = map[string][]string{
	"aws":   {"PCR0", "PCR1", "PCR2"},
	"gcp":   {"image_digest"},
	"azure": {}, // digest(컨테이너) 또는 image_id(VM) — Image 에서 본다
}

// Image — aws 는 파일+sha256, gcp 는 digest, azure 는 digest 또는 image_id.
type Image struct {
	Path    string `json:"path,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Digest  string `json:"digest,omitempty"` // "sha256:<hex>" — 태그 아님
	ImageID string `json:"image_id,omitempty"`
}

type BuildTool struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Registration — 체인에 등록된 위치. 선택.
type Registration struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
}

type MeasurementRecord struct {
	Schema       string         `json:"schema"`
	Service      string         `json:"service"`
	Version      string         `json:"version"`
	Platform     string         `json:"platform"`
	Arch         string         `json:"arch"` // GOARCH the image was built for and measured on: amd64 or arm64. Measurements differ per architecture, so one service/version has one record per architecture.
	Image        Image          `json:"image"`
	Values       p.Measurements `json:"values"`
	BuildTool    BuildTool      `json:"build_tool"`
	Registration *Registration  `json:"registration,omitempty"`
}

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
	case "aws":
		if !validArch(r.Arch) {
			errs = append(errs, "arch: want one of "+strings.Join(archNames(), "|")+" (GOARCH spelling), got "+quote(r.Arch))
		}
		if r.Image.Path == "" {
			errs = append(errs, "image.path: EIF file required for aws")
		}
		if !isHex(r.Image.SHA256, 64) {
			errs = append(errs, "image.sha256: want 64 hex chars")
		}
		if r.BuildTool.Version == "" {
			errs = append(errs, "build_tool.version: required for aws")
		}
	case "gcp":
		if !isDigest(r.Image.Digest) {
			errs = append(errs, "image.digest: want sha256:<64 hex> — a digest, not a tag")
		}
	case "azure":
		if !isDigest(r.Image.Digest) && r.Image.ImageID == "" {
			errs = append(errs, "image: digest (container) or image_id (VM) required for azure")
		}
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

// archNames — 이 프로젝트가 빌드하는 아키텍처의 GOARCH 이름. build.ArchTargets 가 유일한 출처다.
func archNames() []string {
	out := make([]string, 0, len(build.ArchTargets))
	for _, t := range build.ArchTargets {
		out = append(out, t.GOARCH)
	}
	return out
}
