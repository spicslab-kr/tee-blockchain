package vendor

import (
	"context"
	"errors"

	r "tb/internal/tee/request/Vendor"
)

// AzureParser — Azure Confidential VM (MAA JWT) 문서용. 아직 구현되지 않았다.
type AzureParser struct{}

var _ Parser = (*AzureParser)(nil)

func (AzureParser) VerifySig(ctx context.Context, ra r.RADoc, key []byte) error {
	return errors.New("tee/parse/vendor: azure VerifySig not implemented")
}

func (AzureParser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return errors.New("tee/parse/vendor: azure CheckNonce not implemented")
}

func (AzureParser) Parse(ra r.RADoc) (Measurements, error) {
	return nil, errors.New("tee/parse/vendor: azure Parse not implemented")
}
