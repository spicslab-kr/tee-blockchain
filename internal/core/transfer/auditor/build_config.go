// Package auditor 는 파일 전달 — 빌드 설정값과 측정값 기록을 보내고 받는다.
package auditor

const SchemaBuildConfig = "tb.build-config/1"

// TEEInfoKeys — 플랫폼별 필수 TEE 정보 키.
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

type BuildConfig struct {
	Schema   string            `json:"schema"`
	Service  string            `json:"service"`
	Version  string            `json:"version"`
	Platform string            `json:"platform"`
	Source   Source            `json:"source"`
	TEEInfo  map[string]string `json:"tee_info"`
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
	return join(errs)
}
