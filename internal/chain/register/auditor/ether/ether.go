package ether

import (
	"context"
	"errors"

	l "tb/internal/chain/lookup/auditor"
	reg "tb/internal/chain/register/auditor"
	p "tb/internal/tee/parse/auditor"
)

type Registrar struct{}

var _ reg.Registrar = (*Registrar)(nil)

func (Registrar) Sign(values p.Measurements, meta l.Meta, key []byte) (reg.Signed, error) {
	return reg.Signed{}, errors.New("chain/register/auditor/ether: Sign not implemented")
}

func (Registrar) Register(ctx context.Context, s reg.Signed) (string, error) {
	return "", errors.New("chain/register/auditor/ether: Register not implemented")
}
