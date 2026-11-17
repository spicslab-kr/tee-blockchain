package auditor

import (
	"bytes"
	"strings"

	p "attestamp/internal/tee/parse/auditor"
)

const commit = "0123456789abcdef0123456789abcdef01234567"

func cfgAWS() BuildConfig {
	return BuildConfig{Schema: SchemaBuildConfig, Service: "svc", Version: "1.2.0", Platform: "aws-nitro",
		Source:   Source{Repo: "https://example.com/svc.git", Commit: commit},
		TEEInfo:  map[string]string{"instance_id": "i-0abc", "region": "ap-northeast-2", "ami_id": "ami-0123"},
		BuildEnv: envAWSNitro()}
}

// envAWSNitro is envAWS with the fields a build-config.json needs on aws-nitro (the instances of 2026-11-12 and -16).
func envAWSNitro() BuildEnv {
	e := envAWS()
	e.AMIRelease, e.Docker, e.NitroCLI = "2023.12.20261109", "25.0.18-1.amzn2023.0.3", "1.5.0-0.amzn2023"
	return e
}

func envAWS() BuildEnv {
	return BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.26.1", Trimpath: true}
}

func cfgGCP() BuildConfig {
	return BuildConfig{Schema: SchemaBuildConfig, Service: "svc", Version: "1.2.0", Platform: "gcp",
		Source:  Source{Repo: "https://example.com/svc.git", Commit: commit},
		TEEInfo: map[string]string{"instance_name": "svc-1", "project_id": "proj", "container_image": "gcr.io/proj/svc@sha256:" + strings.Repeat("cd", 32)}}
}

func cfgAzure() BuildConfig {
	return BuildConfig{Schema: SchemaBuildConfig, Service: "svc", Version: "1.2.0", Platform: "azure",
		Source:  Source{Repo: "https://example.com/svc.git", Commit: commit},
		TEEInfo: map[string]string{"vm_name": "svc-vm", "region": "koreacentral", "maa_endpoint": "https://svc.kor.attest.azure.net"}}
}

func pcr(fill byte) []byte { return bytes.Repeat([]byte{fill}, 48) }

func recAWS() MeasurementRecord {
	return MeasurementRecord{Schema: SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "aws-nitro", BuildEnv: envAWS(),
		Image:     Image{Path: "svc.eif", SHA256: strings.Repeat("ab", 32)},
		Values:    p.Measurements{{Name: "PCR0", Value: pcr(1)}, {Name: "PCR1", Value: pcr(2)}, {Name: "PCR2", Value: pcr(3)}},
		BuildTool: BuildTool{Name: "nitro-cli", Version: "1.3.0"}}
}

func recGCP() MeasurementRecord {
	d := strings.Repeat("cd", 32)
	return MeasurementRecord{Schema: SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "gcp",
		Image:     Image{Digest: "sha256:" + d},
		Values:    p.Measurements{{Name: "image_digest", Value: bytes.Repeat([]byte{0xcd}, 32)}},
		BuildTool: BuildTool{Name: "docker"}}
}

func recAzure() MeasurementRecord {
	return MeasurementRecord{Schema: SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "azure",
		Image:     Image{ImageID: "/subscriptions/x/resourceGroups/rg/providers/Microsoft.Compute/images/svc-1"},
		Values:    p.Measurements{{Name: "snp_digest", Value: bytes.Repeat([]byte{0xee}, 48)}},
		BuildTool: BuildTool{Name: "packer"}}
}

func cfgTDX() BuildConfig {
	return BuildConfig{Schema: SchemaBuildConfig, Service: "svc", Version: "1.2.0", Platform: "intel-tdx",
		Source:   Source{Repo: "https://example.com/svc.git", Commit: commit},
		TEEInfo:  map[string]string{"instance_name": "svc-tdx", "project_id": "proj", "zone": "us-central1-a", "image": "ubuntu-2404-noble-amd64-v20261101"},
		BuildEnv: envAWS()}
}

func recTDX() MeasurementRecord {
	return MeasurementRecord{Schema: SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "intel-tdx", BuildEnv: envAWS(),
		Image:     Image{Path: "svc-initrd.img", SHA256: strings.Repeat("ab", 32)},
		Values:    p.Measurements{{Name: "MRTD", Value: pcr(1)}, {Name: "RTMR0", Value: pcr(2)}, {Name: "RTMR1", Value: pcr(3)}, {Name: "RTMR2", Value: pcr(4)}},
		BuildTool: BuildTool{Name: "attestamp-cpio"}}
}

func recSNPInitrd() MeasurementRecord {
	return MeasurementRecord{Schema: SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "amd-sev-snp-initrd", BuildEnv: envAWS(),
		Image: Image{Path: "svc-initrd.img", SHA256: strings.Repeat("ab", 32)},
		Values: p.Measurements{{Name: "MEASUREMENT", Value: pcr(1)}, {Name: "PCR4", Value: pcr(2)[:32]}, {Name: "KERNEL", Value: pcr(3)[:32]},
			{Name: "CMDLINE", Value: pcr(4)[:32]}, {Name: "INITRD", Value: pcr(5)[:32]}},
		BuildTool: BuildTool{Name: "attestamp-cpio"}}
}

func recSNPVerity() MeasurementRecord {
	r := recSNPInitrd()
	r.Platform = "amd-sev-snp-verity"
	r.Image.Path = "svc-app.img"
	r.Values = append(r.Values, struct {
		Name  string `json:"name"`
		Value []byte `json:"value"`
	}{Name: "PCR12", Value: pcr(6)[:32]})
	return r
}
