//go:build !linux

package aws

import (
	"context"
	"errors"

	"attestamp/internal/tee/request/auditor"
)

type Extractor struct {
	Device   string
	UserData []byte
}

var _ auditor.Extractor = (*Extractor)(nil)

func (Extractor) Extract(context.Context, auditor.Nonce) (auditor.RADoc, error) {
	return auditor.RADoc{}, errors.New("/dev/nsm is only available inside a Nitro enclave (linux)")
}
