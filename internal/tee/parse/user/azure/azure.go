package azure

import (
	"context"
	"errors"

	p "tb/internal/tee/parse/user"
	r "tb/internal/tee/request/user"
)

type Parser struct{}

var _ p.Parser = (*Parser)(nil)

func (Parser) VerifySig(ctx context.Context, ra r.RADoc, key []byte) error {
	return errors.New("tee/parse/user/azure: VerifySig not implemented")
}

func (Parser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return errors.New("tee/parse/user/azure: CheckNonce not implemented")
}

func (Parser) Parse(ra r.RADoc) (p.Measurements, error) {
	return nil, errors.New("tee/parse/user/azure: Parse not implemented")
}
