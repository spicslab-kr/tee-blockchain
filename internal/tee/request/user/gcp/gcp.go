package gcp

import (
	"context"
	"errors"

	"tb/internal/tee/request/user"
)

type Requester struct{}

var _ user.Requester = (*Requester)(nil)

func (Requester) Request(ctx context.Context, nonce user.Nonce, teeAddr string) (user.RADoc, error) {
	return user.RADoc{}, errors.New("tee/request/user/gcp: not implemented")
}
