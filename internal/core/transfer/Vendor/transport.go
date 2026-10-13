package vendor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Transport sends the build config and receives the measurement record.
type Transport interface {
	Send(ctx context.Context, cfg BuildConfig, to string) (path string, err error)
	Receive(ctx context.Context, from string) (Received, error)
}

// Received holds the parsed value, the original bytes, and the path of the accompanying image file.
type Received struct {
	Value     MeasurementRecord
	Raw       []byte
	Path      string
	SHA256    string
	ImagePath string // image file whose hash has been checked
}

type FileTransport struct{}

var _ Transport = (*FileTransport)(nil)

// Send writes to build-config.json if to is a directory, otherwise to that path.
func (FileTransport) Send(ctx context.Context, cfg BuildConfig, to string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	b, err := Encode(cfg)
	if err != nil {
		return "", err
	}
	dst := resolve(to, "build-config.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := writeAtomic(dst, b); err != nil {
		return "", err
	}
	if err := writeAtomic(dst+".sha256", []byte(Digest(b)+"\n")); err != nil {
		return "", err
	}
	return dst, nil
}

// Receive checks the hashes of the record and image file, then reads them.
func (FileTransport) Receive(ctx context.Context, from string) (Received, error) {
	if err := ctx.Err(); err != nil {
		return Received{}, err
	}
	src := resolve(from, "measurement-record.json")
	raw, err := os.ReadFile(src)
	if err != nil {
		return Received{}, err
	}
	sum := Digest(raw)
	if err := checkSidecar(src, sum); err != nil {
		return Received{}, err
	}
	rec, err := DecodeMeasurementRecord(raw)
	if err != nil {
		return Received{}, err
	}
	out := Received{Value: rec, Raw: raw, Path: src, SHA256: sum}
	if rec.Image.Path != "" {
		img := filepath.Join(filepath.Dir(src), filepath.Base(rec.Image.Path))
		got, err := FileDigest(img)
		if err != nil {
			return Received{}, fmt.Errorf("image file %s: %w", filepath.Base(img), err)
		}
		if got != rec.Image.SHA256 {
			return Received{}, fmt.Errorf("image file %s: sha256 mismatch — file changed in transit", filepath.Base(img))
		}
		out.ImagePath = img
	}
	return out, nil
}

func resolve(target, defaultName string) string {
	if fi, err := os.Stat(target); err == nil && fi.IsDir() {
		return filepath.Join(target, defaultName)
	}
	return target
}

func checkSidecar(path, sum string) error {
	side, err := os.ReadFile(path + ".sha256")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(side)) != sum {
		return fmt.Errorf("%s: sha256 mismatch — file changed in transit", filepath.Base(path))
	}
	return nil
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
