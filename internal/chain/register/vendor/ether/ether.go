package ether

import (
	"context"
	"errors"

	l "tb/internal/chain/lookup/vendor"
	reg "tb/internal/chain/register/vendor"
	p "tb/internal/tee/parse/vendor"
)

type Registrar struct{}

var _ reg.Registrar = (*Registrar)(nil)

func (Registrar) Sign(values p.Measurements, meta l.Meta, key []byte) (reg.Signed, error) {
	return reg.Signed{}, errors.New("chain/register/vendor/ether: Sign not implemented")
}

func (Registrar) Register(ctx context.Context, s reg.Signed) (string, error) {
	return "", errors.New("chain/register/vendor/ether: Register not implemented")
}
