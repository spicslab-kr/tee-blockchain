package azure

import (
	"context"
	"errors"

	"tb/internal/tee/request/vendor"
)

type Requester struct{}

var _ vendor.Requester = (*Requester)(nil)

func (Requester) Request(ctx context.Context, nonce vendor.Nonce, teeAddr string) (vendor.RADoc, error) {
	return vendor.RADoc{}, errors.New("tee/request/vendor/azure: not implemented")
}
