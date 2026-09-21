package gcp

import (
	"context"
	"errors"

	"tb/internal/tee/request/Vendor"
)

type Requester struct{}

var _ vendor.Requester = (*Requester)(nil)

func (Requester) Request(ctx context.Context, nonce vendor.Nonce, teeAddr string) (vendor.RADoc, error) {
	return vendor.RADoc{}, errors.New("tee/request/vendor/gcp: not implemented")
}
