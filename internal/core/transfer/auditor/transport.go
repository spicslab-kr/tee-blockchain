package auditor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Transport receives the build config and sends the measurement record and image.
type Transport interface {
	Receive(ctx context.Context, from string) (Received, error)
	Send(ctx context.Context, rec MeasurementRecord, to string) (path string, err error)
}

// Received holds the parsed value and the original bytes.
type Received struct {
	Value  BuildConfig
	Raw    []byte
	Path   string
	SHA256 string
}

type FileTransport struct{}

var _ Transport = (*FileTransport)(nil)

// Receive checks the hash, then reads.
func (FileTransport) Receive(ctx context.Context, from string) (Received, error) {
	if err := ctx.Err(); err != nil {
		return Received{}, err
	}
	src := resolve(from, "build-config.json")
	raw, err := os.ReadFile(src)
	if err != nil {
		return Received{}, err
	}
	sum := Digest(raw)
	if err := checkSidecar(src, sum); err != nil {
		return Received{}, err
	}
	cfg, err := DecodeBuildConfig(raw)
	if err != nil {
		return Received{}, err
	}
	return Received{Value: cfg, Raw: raw, Path: src, SHA256: sum}, nil
}

// Send writes the record and, if there is an image file, copies it into the same directory. The record's sha256 must match the file.
func (FileTransport) Send(ctx context.Context, rec MeasurementRecord, to string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dst := resolve(to, "measurement-record.json")
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if rec.Image.Path != "" {
		sum, err := FileDigest(rec.Image.Path)
		if err != nil {
			return "", fmt.Errorf("image file: %w", err)
		}
		if rec.Image.SHA256 == "" {
			rec.Image.SHA256 = sum
		} else if rec.Image.SHA256 != sum {
			return "", fmt.Errorf("image file %s: sha256 in record does not match the file", filepath.Base(rec.Image.Path))
		}
		base := filepath.Base(rec.Image.Path)
		if err := copyFile(rec.Image.Path, filepath.Join(dir, base)); err != nil {
			return "", err
		}
		rec.Image.Path = base
	}
	if err := rec.Validate(); err != nil {
		return "", err
	}
	b, err := Encode(rec)
	if err != nil {
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

func copyFile(src, dst string) error {
	if filepath.Clean(src) == filepath.Clean(dst) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
