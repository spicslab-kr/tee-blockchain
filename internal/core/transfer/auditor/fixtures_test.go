package auditor

import (
	"bytes"
	"strings"

	p "tb/internal/tee/parse/auditor"
)

const commit = "0123456789abcdef0123456789abcdef01234567"

func cfgAWS() BuildConfig {
	return BuildConfig{Schema: SchemaBuildConfig, Service: "svc", Version: "1.2.0", Platform: "aws",
		Source:  Source{Repo: "https://example.com/svc.git", Commit: commit},
		TEEInfo: map[string]string{"instance_id": "i-0abc", "region": "ap-northeast-2", "ami_id": "ami-0123"}}
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
	return MeasurementRecord{Schema: SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "aws",
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
