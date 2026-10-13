package azure

import (
	"context"
	"errors"

	"tb/internal/tee/request/Vendor"
)

const Format = "azure-maa-jwt"

// Extractor reads the HCL report from the vTPM, sends it to MAA and receives a JWT. It needs go-tpm and is still empty.
type Extractor struct {
	MAAEndpoint string
}

var _ vendor.Extractor = (*Extractor)(nil)

func (Extractor) Extract(context.Context, vendor.Nonce) (vendor.RADoc, error) {
	return vendor.RADoc{}, errors.New("azure extractor not implemented — needs vTPM (go-tpm) and MAA")
}
