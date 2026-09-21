// Package auditor 는 C1 체인 등록 — 서명해서 컨트랙트에 기록.
package auditor

import (
	"context"

	l "tb/internal/chain/lookup/auditor"
	p "tb/internal/tee/parse/auditor"
)

type Signed struct {
	Values    p.Measurements
	Meta      l.Meta
	Signer    string
	Signature []byte
}

type Registrar interface {
	Sign(values p.Measurements, meta l.Meta, key []byte) (Signed, error)
	Register(ctx context.Context, s Signed) (address string, err error)
}
