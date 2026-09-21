// Package vendor 는 C2 체인 조회 — 읽기만 한다.
package vendor

import (
	"context"

	"tb/internal/tee/parse/Vendor"
)

type Meta struct {
	Service  string
	Version  string
	Platform string
}

type Registration struct {
	Address   string
	Values    vendor.Measurements
	Meta      Meta
	Expires   int64 // 체인 시계 기준
	Signer    string
	Signature []byte
}

type Reader interface {
	FindAddress(ctx context.Context, values vendor.Measurements) ([]string, error)
	Lookup(ctx context.Context, address string) (Registration, error)
}
