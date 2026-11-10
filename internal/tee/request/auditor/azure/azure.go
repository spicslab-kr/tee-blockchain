package azure

import (
	"context"
	"errors"

	"attestamp/internal/tee/request/auditor"
)

const Format = "azure-maa-jwt"

// Extractor reads the HCL report from the vTPM, sends it to MAA and receives a JWT. Extract returns an error: the azure-maa-jwt request side is scaffolding.
type Extractor struct {
	MAAEndpoint string
}

var _ auditor.Extractor = (*Extractor)(nil)

func (Extractor) Extract(context.Context, auditor.Nonce) (auditor.RADoc, error) {
	return auditor.RADoc{}, errors.New("azure extractor not implemented: azure-maa-jwt requests are scaffolding")
}
