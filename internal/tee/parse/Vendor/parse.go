// Package vendor 는 T2 RA 파싱 — 서명·난수를 확인하고 측정값만 꺼낸다.
package vendor

import (
	"context"

	"tb/internal/tee/request/Vendor"
)

type Measurement struct {
	Name  string
	Value []byte
}

type Measurements []Measurement

type Parser interface {
	VerifySig(ctx context.Context, ra vendor.RADoc, key []byte) error
	CheckNonce(ra vendor.RADoc, nonce vendor.Nonce) error
	Parse(ra vendor.RADoc) (Measurements, error)
}
