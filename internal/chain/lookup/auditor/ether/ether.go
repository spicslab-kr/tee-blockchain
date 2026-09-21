package ether

import (
	"context"
	"errors"

	l "tb/internal/chain/lookup/auditor"
	p "tb/internal/tee/parse/auditor"
)

type Reader struct{}

var _ l.Reader = (*Reader)(nil)

func (Reader) FindAddress(ctx context.Context, values p.Measurements) ([]string, error) {
	return nil, errors.New("chain/lookup/auditor/ether: FindAddress not implemented")
}

func (Reader) Lookup(ctx context.Context, address string) (l.Registration, error) {
	return l.Registration{}, errors.New("chain/lookup/auditor/ether: Lookup not implemented")
}
