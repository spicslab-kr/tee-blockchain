package aws

import (
	"context"
	"errors"

	p "tb/internal/tee/parse/vendor"
	r "tb/internal/tee/request/vendor"
)

type Parser struct{}

var _ p.Parser = (*Parser)(nil)

func (Parser) VerifySig(ctx context.Context, ra r.RADoc, key []byte) error {
	return errors.New("tee/parse/vendor/aws: VerifySig not implemented")
}

func (Parser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return errors.New("tee/parse/vendor/aws: CheckNonce not implemented")
}

func (Parser) Parse(ra r.RADoc) (p.Measurements, error) {
	return nil, errors.New("tee/parse/vendor/aws: Parse not implemented")
}
