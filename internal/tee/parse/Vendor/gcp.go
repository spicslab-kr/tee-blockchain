package vendor

import (
	"context"
	"errors"

	r "attestamp/internal/tee/request/Vendor"
)

// GCPParser is for GCP Confidential Space (JWT) documents. Not implemented yet.
type GCPParser struct{}

var _ Parser = (*GCPParser)(nil)

func (GCPParser) VerifySig(ctx context.Context, ra r.RADoc, rootCert []byte) error {
	return errors.New("tee/parse/vendor: gcp VerifySig not implemented")
}

func (GCPParser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return errors.New("tee/parse/vendor: gcp CheckNonce not implemented")
}

func (GCPParser) Parse(ra r.RADoc) (Measurements, error) {
	return nil, errors.New("tee/parse/vendor: gcp Parse not implemented")
}
