// Package auditor 는 T2 RA 파싱 — 서명·난수를 확인하고 측정값만 꺼낸다.
package auditor

import (
	"context"

	"tb/internal/tee/request/auditor"
)

type Measurement struct {
	Name  string
	Value []byte
}

type Measurements []Measurement

type Parser interface {
	VerifySig(ctx context.Context, ra auditor.RADoc, key []byte) error
	CheckNonce(ra auditor.RADoc, nonce auditor.Nonce) error
	Parse(ra auditor.RADoc) (Measurements, error)
}
