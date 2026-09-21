// Package user 는 T2 RA 파싱 — 서명·난수를 확인하고 측정값만 꺼낸다.
package user

import (
	"context"

	"tb/internal/tee/request/user"
)

type Measurement struct {
	Name  string
	Value []byte
}

type Measurements []Measurement

type Parser interface {
	VerifySig(ctx context.Context, ra user.RADoc, key []byte) error
	CheckNonce(ra user.RADoc, nonce user.Nonce) error
	Parse(ra user.RADoc) (Measurements, error)
}
