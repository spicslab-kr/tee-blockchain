package auditor

import (
	"context"
	"errors"

	r "attestamp/internal/tee/request/auditor"
)

// GCPParser is a placeholder for GCP Confidential Space (JWT) documents. Every method returns an error: these documents cannot be verified yet.
type GCPParser struct{}

var _ Parser = (*GCPParser)(nil)

func (GCPParser) VerifySig(ctx context.Context, ra r.RADoc, rootCert []byte) error {
	return errors.New("tee/parse/auditor: gcp VerifySig not implemented")
}

func (GCPParser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return errors.New("tee/parse/auditor: gcp CheckNonce not implemented")
}

func (GCPParser) Parse(ra r.RADoc) (Measurements, error) {
	return nil, errors.New("tee/parse/auditor: gcp Parse not implemented")
}
