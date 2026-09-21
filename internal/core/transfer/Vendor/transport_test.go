package vendor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSend_BuildConfig_FileAndSidecar(t *testing.T) {
	dir := t.TempDir()
	path, err := (FileTransport{}).Send(context.Background(), cfgAWS(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "build-config.json" {
		t.Fatalf("default file name: %s", path)
	}
	raw, _ := os.ReadFile(path)
	side, err := os.ReadFile(path + ".sha256")
	if err != nil || strings.TrimSpace(string(side)) != Digest(raw) {
		t.Fatalf("sidecar wrong: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file must not remain")
	}
	// 파일 경로를 직접 주면 그 이름으로
	p2, err := (FileTransport{}).Send(context.Background(), cfgGCP(), filepath.Join(dir, "out", "cfg.json"))
	if err != nil || filepath.Base(p2) != "cfg.json" {
		t.Fatalf("explicit path: %s %v", p2, err)
	}
}

func TestSend_RefusesInvalidAndCancelled(t *testing.T) {
	c := cfgAWS()
	delete(c.TEEInfo, "ami_id")
	if _, err := (FileTransport{}).Send(context.Background(), c, t.TempDir()); err == nil {
		t.Fatal("invalid config must not be sent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (FileTransport{}).Send(ctx, cfgAWS(), t.TempDir()); err == nil {
		t.Fatal("cancelled context must stop send")
	}
}

func writeRecord(t *testing.T, dir string, r MeasurementRecord, withSidecar bool) []byte {
	t.Helper()
	b, _ := Encode(r)
	if err := os.WriteFile(filepath.Join(dir, "measurement-record.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if withSidecar {
		_ = os.WriteFile(filepath.Join(dir, "measurement-record.json.sha256"), []byte(Digest(b)), 0o644)
	}
	return b
}

func writeImage(t *testing.T, dir string, r *MeasurementRecord, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, r.Image.Path), content, 0o644); err != nil {
		t.Fatal(err)
	}
	r.Image.SHA256 = Digest(content)
}

func TestReceive_AWS_RecordAndImage(t *testing.T) {
	dir := t.TempDir()
	r := recAWS()
	writeImage(t, dir, &r, []byte("eif-bytes"))
	b := writeRecord(t, dir, r, true)
	got, err := (FileTransport{}).Receive(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Raw) != string(b) {
		t.Error("Raw must be the untouched bytes")
	}
	if got.ImagePath != filepath.Join(dir, "svc.eif") {
		t.Errorf("image path: %s", got.ImagePath)
	}
	if got.Value.BuildTool.Version != "1.3.0" || len(got.Value.Values) != 3 {
		t.Errorf("decoded record wrong: %+v", got.Value)
	}
}

func TestReceive_GCPAndAzure_NoImageFile(t *testing.T) {
	for _, r := range []MeasurementRecord{recGCP(), recAzure()} {
		dir := t.TempDir()
		writeRecord(t, dir, r, true)
		got, err := (FileTransport{}).Receive(context.Background(), dir)
		if err != nil {
			t.Fatalf("%s: %v", r.Platform, err)
		}
		if got.ImagePath != "" {
			t.Errorf("%s: no image file expected", r.Platform)
		}
	}
}

func TestReceive_RecordTamperedIsRejected(t *testing.T) {
	dir := t.TempDir()
	r := recGCP()
	b := writeRecord(t, dir, r, true)
	tampered := strings.Replace(string(b), `"1.2.0"`, `"1.2.1"`, 1)
	_ = os.WriteFile(filepath.Join(dir, "measurement-record.json"), []byte(tampered), 0o644)
	if _, err := (FileTransport{}).Receive(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want sha256 mismatch, got %v", err)
	}
}

func TestReceive_ImageTamperedIsRejected(t *testing.T) {
	dir := t.TempDir()
	r := recAWS()
	writeImage(t, dir, &r, []byte("eif-bytes"))
	writeRecord(t, dir, r, true)
	_ = os.WriteFile(filepath.Join(dir, "svc.eif"), []byte("eif-bytes-changed"), 0o644)
	if _, err := (FileTransport{}).Receive(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "image file") {
		t.Fatalf("want image mismatch, got %v", err)
	}
}

func TestReceive_ImageMissingIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeRecord(t, dir, recAWS(), true)
	if _, err := (FileTransport{}).Receive(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "image file") {
		t.Fatalf("want missing image error, got %v", err)
	}
}

func TestReceive_NoSidecarStillReads(t *testing.T) {
	dir := t.TempDir()
	writeRecord(t, dir, recGCP(), false)
	if _, err := (FileTransport{}).Receive(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
}

func TestReceive_MissingFileAndCancelled(t *testing.T) {
	if _, err := (FileTransport{}).Receive(context.Background(), t.TempDir()); err == nil {
		t.Fatal("missing record must error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (FileTransport{}).Receive(ctx, t.TempDir()); err == nil {
		t.Fatal("cancelled context must stop receive")
	}
}
