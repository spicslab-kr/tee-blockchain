package ether

import (
	"context"
	"errors"

	l "tb/internal/chain/lookup/user"
	p "tb/internal/tee/parse/user"
)

type Reader struct{}

var _ l.Reader = (*Reader)(nil)

func (Reader) FindAddress(ctx context.Context, values p.Measurements) ([]string, error) {
	return nil, errors.New("chain/lookup/user/ether: FindAddress not implemented")
}

func (Reader) Lookup(ctx context.Context, address string) (l.Registration, error) {
	return l.Registration{}, errors.New("chain/lookup/user/ether: Lookup not implemented")
}
