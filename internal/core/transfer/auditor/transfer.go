// Package auditor 는 M1 전달 — Auditor 쪽. 빌드 설정값을 받고 측정값 기록을 보낸다.
package auditor

import (
	"context"
	"errors"

	p "tb/internal/tee/parse/auditor"
)

// BuildConfig 는 Vendor → Auditor. 받은 그대로 보관해 M2 기준값으로 쓴다.
type BuildConfig struct {
	SourceRepo string
	Commit     string
	Platform   string
	TEEInfo    map[string]string
}

// MeasurementRecord 는 Auditor → Vendor.
type MeasurementRecord struct {
	ImagePath string
	Values    p.Measurements
	BuildTool string // setup/auditor/build.md 가 적으라는 값
}

type Transport interface {
	Receive(ctx context.Context, from string) (BuildConfig, error)
	Send(ctx context.Context, rec MeasurementRecord, to string) error
}

type FileTransport struct{}

var _ Transport = (*FileTransport)(nil)

func (FileTransport) Receive(ctx context.Context, from string) (BuildConfig, error) {
	return BuildConfig{}, errors.New("core/transfer/auditor: Receive not implemented")
}

func (FileTransport) Send(ctx context.Context, rec MeasurementRecord, to string) error {
	return errors.New("core/transfer/auditor: Send not implemented")
}
