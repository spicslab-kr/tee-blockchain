//go:build !linux

package aws

import (
	"context"
	"errors"

	"tb/internal/tee/request/Vendor"
)

type Extractor struct {
	Device   string
	UserData []byte
}

var _ vendor.Extractor = (*Extractor)(nil)

func (Extractor) Extract(context.Context, vendor.Nonce) (vendor.RADoc, error) {
	return vendor.RADoc{}, errors.New("/dev/nsm is only available inside a Nitro enclave (linux)")
}
