package vendor

import (
	"context"
	"errors"

	r "tb/internal/tee/request/Vendor"
)

// GCPParser — GCP Confidential Space (JWT) 문서용. 아직 구현되지 않았다.
type GCPParser struct{}

var _ Parser = (*GCPParser)(nil)

func (GCPParser) VerifySig(ctx context.Context, ra r.RADoc, key []byte) error {
	return errors.New("tee/parse/vendor: gcp VerifySig not implemented")
}

func (GCPParser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return errors.New("tee/parse/vendor: gcp CheckNonce not implemented")
}

func (GCPParser) Parse(ra r.RADoc) (Measurements, error) {
	return nil, errors.New("tee/parse/vendor: gcp Parse not implemented")
}
