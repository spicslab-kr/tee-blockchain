package auditor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Encode — 들여쓰기 고정.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeBuildConfig(b []byte) (BuildConfig, error) {
	var c BuildConfig
	if err := decodeStrict(b, &c); err != nil {
		return BuildConfig{}, fmt.Errorf("build config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return BuildConfig{}, fmt.Errorf("build config: %w", err)
	}
	return c, nil
}

func DecodeMeasurementRecord(b []byte) (MeasurementRecord, error) {
	var r MeasurementRecord
	if err := decodeStrict(b, &r); err != nil {
		return MeasurementRecord{}, fmt.Errorf("measurement record: %w", err)
	}
	if err := r.Validate(); err != nil {
		return MeasurementRecord{}, fmt.Errorf("measurement record: %w", err)
	}
	return r, nil
}

// decodeStrict — 모르는 필드는 거절한다.
func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func quote(s string) string { return `"` + s + `"` }

func join(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, "; "))
}
