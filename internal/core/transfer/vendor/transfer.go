// Package vendor 는 M1 전달 — Vendor 쪽. 빌드 설정값을 보내고 측정값 기록을 받는다.
package vendor

import (
	"context"
	"errors"

	p "tb/internal/tee/parse/vendor"
)

// BuildConfig 는 Vendor → Auditor.
type BuildConfig struct {
	SourceRepo string
	Commit     string
	Platform   string
	TEEInfo    map[string]string // setup/vendor/launch.md 가 적으라는 값
}

// MeasurementRecord 는 Auditor → Vendor. 받은 그대로 보관해 M2 기준값으로 쓴다.
type MeasurementRecord struct {
	ImagePath string
	Values    p.Measurements
	BuildTool string
}

type Transport interface {
	Send(ctx context.Context, cfg BuildConfig, to string) error
	Receive(ctx context.Context, from string) (MeasurementRecord, error)
}

type FileTransport struct{}

var _ Transport = (*FileTransport)(nil)

func (FileTransport) Send(ctx context.Context, cfg BuildConfig, to string) error {
	return errors.New("core/transfer/vendor: Send not implemented")
}

func (FileTransport) Receive(ctx context.Context, from string) (MeasurementRecord, error) {
	return MeasurementRecord{}, errors.New("core/transfer/vendor: Receive not implemented")
}
