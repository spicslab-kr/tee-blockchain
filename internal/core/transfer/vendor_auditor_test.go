// Vendor <-> Auditor transfer: what one side Sends the other Receives. This runs the Vendor-to-Auditor source handoff and the Auditor-to-Vendor image handoff as they happen in practice.
package transfer_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	vendor "attestamp/internal/core/transfer/Vendor"
	auditor "attestamp/internal/core/transfer/auditor"
	ap "attestamp/internal/tee/parse/auditor"
)

const commit = "0123456789abcdef0123456789abcdef01234567"

// The Vendor sends source plus TEE info and the Auditor receives it, on all three platforms.
func TestTransfer_VendorToAuditor_BuildConfig(t *testing.T) {
	cases := []vendor.BuildConfig{
		{Schema: vendor.SchemaBuildConfig, Service: "svc", Version: "1.2.0", Platform: "aws",
			Source:   vendor.Source{Repo: "https://example.com/svc.git", Commit: commit},
			TEEInfo:  map[string]string{"instance_id": "i-0abc", "region": "ap-northeast-2", "ami_id": "ami-0123"},
			BuildEnv: vendor.BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.26.1", Trimpath: true}},
		{Schema: vendor.SchemaBuildConfig, Service: "svc", Version: "1.2.0", Platform: "gcp",
			Source:  vendor.Source{Repo: "https://example.com/svc.git", Commit: commit},
			TEEInfo: map[string]string{"instance_name": "svc-1", "project_id": "proj", "container_image": "gcr.io/proj/svc@sha256:" + strings.Repeat("cd", 32)}},
		{Schema: vendor.SchemaBuildConfig, Service: "svc", Version: "1.2.0", Platform: "azure",
			Source:  vendor.Source{Repo: "https://example.com/svc.git", Commit: commit},
			TEEInfo: map[string]string{"vm_name": "svc-vm", "region": "koreacentral", "maa_endpoint": "https://svc.kor.attest.azure.net"}},
	}
	for _, cfg := range cases {
		dir := t.TempDir()
		if _, err := (vendor.FileTransport{}).Send(context.Background(), cfg, dir); err != nil {
			t.Fatalf("%s send: %v", cfg.Platform, err)
		}
		got, err := (auditor.FileTransport{}).Receive(context.Background(), dir)
		if err != nil {
			t.Fatalf("%s receive: %v", cfg.Platform, err)
		}
		if got.Value.Platform != cfg.Platform || got.Value.Source.Commit != commit {
			t.Errorf("%s: auditor got a different config: %+v", cfg.Platform, got.Value)
		}
		for k, v := range cfg.TEEInfo {
			if got.Value.TEEInfo[k] != v {
				t.Errorf("%s: tee_info.%s = %q, want %q", cfg.Platform, k, got.Value.TEEInfo[k], v)
			}
		}
		if got.Value.BuildEnv != auditor.BuildEnv(cfg.BuildEnv) {
			t.Errorf("%s: build_env = %+v, want %+v", cfg.Platform, got.Value.BuildEnv, cfg.BuildEnv)
		}
	}
}

// The Auditor sends the image file it built, the measurements and the registration, and the Vendor receives them.
// The Vendor keeps the received original as is (the reference value for comparison). The image hash must match the record.
func TestTransfer_AuditorToVendor_MeasurementRecordWithImage(t *testing.T) {
	build := t.TempDir()
	eif := filepath.Join(build, "svc.eif")
	if err := os.WriteFile(eif, []byte("eif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := auditor.MeasurementRecord{
		Schema: auditor.SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "aws", BuildEnv: auditor.BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.26.1", Trimpath: true},
		Image: auditor.Image{Path: eif},
		Values: ap.Measurements{
			{Name: "PCR0", Value: bytes.Repeat([]byte{1}, 48)},
			{Name: "PCR1", Value: bytes.Repeat([]byte{2}, 48)},
			{Name: "PCR2", Value: bytes.Repeat([]byte{3}, 48)},
		},
		BuildTool:    auditor.BuildTool{Name: "nitro-cli", Version: "1.3.0"},
		Registration: &auditor.Registration{Chain: "ether", Address: "0xabc"},
	}
	handoff := t.TempDir()
	if _, err := (auditor.FileTransport{}).Send(context.Background(), rec, handoff); err != nil {
		t.Fatal(err)
	}
	got, err := (vendor.FileTransport{}).Receive(context.Background(), handoff)
	if err != nil {
		t.Fatal(err)
	}
	img, _ := os.ReadFile(got.ImagePath)
	if string(img) != "eif-bytes" {
		t.Error("vendor did not get the same image file")
	}
	if got.Value.Image.SHA256 != vendor.Digest([]byte("eif-bytes")) {
		t.Error("record sha256 must be the image file hash")
	}
	if len(got.Value.Values) != 3 || got.Value.Values[2].Name != "PCR2" {
		t.Errorf("measurements changed: %+v", got.Value.Values)
	}
	if got.Value.Registration == nil || got.Value.Registration.Address != "0xabc" {
		t.Errorf("registration lost: %+v", got.Value.Registration)
	}
	// Original kept: Raw equals the file bytes and matches the sidecar hash
	disk, _ := os.ReadFile(got.Path)
	if !bytes.Equal(got.Raw, disk) {
		t.Error("Raw must equal the file on disk")
	}
}

// On GCP and Azure the image is a digest / ID, so only the record is sent, with no file.
func TestTransfer_AuditorToVendor_DigestOnlyPlatforms(t *testing.T) {
	recs := []auditor.MeasurementRecord{
		{Schema: auditor.SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "gcp",
			Image:     auditor.Image{Digest: "sha256:" + strings.Repeat("cd", 32)},
			Values:    ap.Measurements{{Name: "image_digest", Value: bytes.Repeat([]byte{0xcd}, 32)}},
			BuildTool: auditor.BuildTool{Name: "docker"}},
		{Schema: auditor.SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "azure",
			Image:     auditor.Image{ImageID: "/subscriptions/x/images/svc-1"},
			Values:    ap.Measurements{{Name: "snp_digest", Value: bytes.Repeat([]byte{0xee}, 48)}},
			BuildTool: auditor.BuildTool{Name: "packer"}},
	}
	for _, rec := range recs {
		dir := t.TempDir()
		if _, err := (auditor.FileTransport{}).Send(context.Background(), rec, dir); err != nil {
			t.Fatalf("%s: %v", rec.Platform, err)
		}
		got, err := (vendor.FileTransport{}).Receive(context.Background(), dir)
		if err != nil {
			t.Fatalf("%s: %v", rec.Platform, err)
		}
		if got.ImagePath != "" || got.Value.Image.Digest != rec.Image.Digest || got.Value.Image.ImageID != rec.Image.ImageID {
			t.Errorf("%s: image fields changed: %+v", rec.Platform, got.Value.Image)
		}
	}
}

// If it changes in transit the receiver rejects it, for both the record file and the image file.
func TestTransfer_TamperInTransitIsCaught(t *testing.T) {
	build := t.TempDir()
	eif := filepath.Join(build, "svc.eif")
	_ = os.WriteFile(eif, []byte("eif-bytes"), 0o644)
	rec := auditor.MeasurementRecord{
		Schema: auditor.SchemaMeasurementRecord, Service: "svc", Version: "1.2.0", Platform: "aws", BuildEnv: auditor.BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.26.1", Trimpath: true},
		Image:     auditor.Image{Path: eif},
		Values:    ap.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{1}, 48)}, {Name: "PCR1", Value: bytes.Repeat([]byte{2}, 48)}, {Name: "PCR2", Value: bytes.Repeat([]byte{3}, 48)}},
		BuildTool: auditor.BuildTool{Name: "nitro-cli", Version: "1.3.0"},
	}
	// Swap the image
	dir := t.TempDir()
	_, _ = (auditor.FileTransport{}).Send(context.Background(), rec, dir)
	_ = os.WriteFile(filepath.Join(dir, "svc.eif"), []byte("evil"), 0o644)
	if _, err := (vendor.FileTransport{}).Receive(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "image file") {
		t.Fatalf("swapped image must be caught, got %v", err)
	}
	// Tamper with the record (one character of a PCR value)
	dir = t.TempDir()
	path, _ := (auditor.FileTransport{}).Send(context.Background(), rec, dir)
	raw, _ := os.ReadFile(path)
	_ = os.WriteFile(path, bytes.Replace(raw, []byte("AQEB"), []byte("AQEC"), 1), 0o644)
	if _, err := (vendor.FileTransport{}).Receive(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("edited record must be caught, got %v", err)
	}
}
