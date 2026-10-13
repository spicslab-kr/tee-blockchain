package auditor

import (
	"context"
	"errors"

	r "attestamp/internal/tee/request/auditor"
)

// AzureParser is for Azure Confidential VM (MAA JWT) documents. Not implemented yet.
type AzureParser struct{}

var _ Parser = (*AzureParser)(nil)

func (AzureParser) VerifySig(ctx context.Context, ra r.RADoc, rootCert []byte) error {
	return errors.New("tee/parse/auditor: azure VerifySig not implemented")
}

func (AzureParser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return errors.New("tee/parse/auditor: azure CheckNonce not implemented")
}

func (AzureParser) Parse(ra r.RADoc) (Measurements, error) {
	return nil, errors.New("tee/parse/auditor: azure Parse not implemented")
}
