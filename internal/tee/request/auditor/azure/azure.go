package azure

import (
	"context"
	"errors"

	"tb/internal/tee/request/auditor"
)

type Requester struct{}

var _ auditor.Requester = (*Requester)(nil)

func (Requester) Request(ctx context.Context, nonce auditor.Nonce, teeAddr string) (auditor.RADoc, error) {
	return auditor.RADoc{}, errors.New("tee/request/auditor/azure: not implemented")
}
