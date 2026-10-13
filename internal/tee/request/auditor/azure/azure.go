package azure

import (
	"context"
	"errors"

	"attestamp/internal/tee/request/auditor"
)

const Format = "azure-maa-jwt"

// Extractor reads the HCL report from the vTPM, sends it to MAA and receives a JWT. It needs go-tpm and is still empty.
type Extractor struct {
	MAAEndpoint string
}

var _ auditor.Extractor = (*Extractor)(nil)

func (Extractor) Extract(context.Context, auditor.Nonce) (auditor.RADoc, error) {
	return auditor.RADoc{}, errors.New("azure extractor not implemented — needs vTPM (go-tpm) and MAA")
}
