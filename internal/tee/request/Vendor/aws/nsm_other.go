//go:build !linux

package aws

import (
	"context"
	"errors"

	"attestamp/internal/tee/request/Vendor"
)

// Extractor is a stub on this platform; Extract always returns an error.
type Extractor struct {
	Device   string
	UserData []byte
}

var _ vendor.Extractor = (*Extractor)(nil)

func (Extractor) Extract(context.Context, vendor.Nonce) (vendor.RADoc, error) {
	return vendor.RADoc{}, errors.New("/dev/nsm is only available inside a Nitro enclave (linux)")
}
