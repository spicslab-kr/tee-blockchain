package auditor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir string, c BuildConfig, withSidecar bool) []byte {
	t.Helper()
	b, _ := Encode(c)
	if err := os.WriteFile(filepath.Join(dir, "build-config.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if withSidecar {
		_ = os.WriteFile(filepath.Join(dir, "build-config.json.sha256"), []byte(Digest(b)), 0o644)
	}
	return b
}

func TestReceive_BuildConfig_AllPlatforms(t *testing.T) {
	for _, c := range []BuildConfig{cfgAWS(), cfgGCP(), cfgAzure()} {
		dir := t.TempDir()
		b := writeConfig(t, dir, c, true)
		got, err := (FileTransport{}).Receive(context.Background(), dir)
		if err != nil {
			t.Fatalf("%s: %v", c.Platform, err)
		}
		if string(got.Raw) != string(b) {
			t.Errorf("%s: Raw must be untouched", c.Platform)
		}
		for k, v := range c.TEEInfo {
			if got.Value.TEEInfo[k] != v {
				t.Errorf("%s: tee_info.%s lost", c.Platform, k)
			}
		}
	}
}

func TestReceive_TamperedIsRejected(t *testing.T) {
	dir := t.TempDir()
	b := writeConfig(t, dir, cfgAWS(), true)
	_ = os.WriteFile(filepath.Join(dir, "build-config.json"), []byte(strings.Replace(string(b), "ami-0123", "ami-9999", 1)), 0o644)
	if _, err := (FileTransport{}).Receive(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want sha256 mismatch, got %v", err)
	}
}

func TestReceive_InvalidConfigRejected(t *testing.T) {
	dir := t.TempDir()
	c := cfgAWS()
	delete(c.TEEInfo, "region")
	writeConfig(t, dir, c, false)
	if _, err := (FileTransport{}).Receive(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "tee_info.region") {
		t.Fatalf("want tee_info.region error, got %v", err)
	}
}

func TestSend_AWS_CopiesImageAndFillsHash(t *testing.T) {
	src := t.TempDir()
	eif := filepath.Join(src, "svc.eif")
	if err := os.WriteFile(eif, []byte("eif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := recAWS()
	r.Image = Image{Path: eif} // 해시는 Send 가 채운다
	dst := t.TempDir()
	path, err := (FileTransport{}).Send(context.Background(), r, dst)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(filepath.Join(dst, "svc.eif"))
	if err != nil || string(copied) != "eif-bytes" {
		t.Fatalf("image not copied next to record: %v", err)
	}
	raw, _ := os.ReadFile(path)
	got, err := DecodeMeasurementRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Image.Path != "svc.eif" || got.Image.SHA256 != Digest([]byte("eif-bytes")) {
		t.Errorf("record image fields: %+v", got.Image)
	}
	if _, err := os.Stat(filepath.Join(dst, "svc.eif.tmp")); !os.IsNotExist(err) {
		t.Fatal("temp image must not remain")
	}
}

func TestSend_AWS_HashMismatchRefused(t *testing.T) {
	src := t.TempDir()
	eif := filepath.Join(src, "svc.eif")
	_ = os.WriteFile(eif, []byte("eif-bytes"), 0o644)
	r := recAWS()
	r.Image = Image{Path: eif, SHA256: strings.Repeat("00", 32)}
	if _, err := (FileTransport{}).Send(context.Background(), r, t.TempDir()); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("want hash mismatch, got %v", err)
	}
}

func TestSend_AWS_MissingImageRefused(t *testing.T) {
	r := recAWS()
	r.Image = Image{Path: filepath.Join(t.TempDir(), "nope.eif")}
	if _, err := (FileTransport{}).Send(context.Background(), r, t.TempDir()); err == nil {
		t.Fatal("missing image file must refuse send")
	}
}

func TestSend_GCPAndAzure_NoImageFile(t *testing.T) {
	for _, r := range []MeasurementRecord{recGCP(), recAzure()} {
		dir := t.TempDir()
		path, err := (FileTransport{}).Send(context.Background(), r, dir)
		if err != nil {
			t.Fatalf("%s: %v", r.Platform, err)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 2 { // record + sidecar
			t.Errorf("%s: want 2 files, got %d", r.Platform, len(entries))
		}
		if filepath.Base(path) != "measurement-record.json" {
			t.Errorf("%s: default name: %s", r.Platform, path)
		}
	}
}

func TestSend_WithRegistration(t *testing.T) {
	r := recGCP()
	r.Registration = &Registration{Chain: "ether", Address: "0xabc"}
	dir := t.TempDir()
	path, err := (FileTransport{}).Send(context.Background(), r, dir)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"address": "0xabc"`) {
		t.Fatal("registration must be written")
	}
}

func TestSend_RefusesInvalidAndCancelled(t *testing.T) {
	r := recGCP()
	r.Image.Digest = "gcr.io/proj/svc:latest"
	if _, err := (FileTransport{}).Send(context.Background(), r, t.TempDir()); err == nil {
		t.Fatal("tag instead of digest must not be sent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (FileTransport{}).Send(ctx, recGCP(), t.TempDir()); err == nil {
		t.Fatal("cancelled context must stop send")
	}
}
