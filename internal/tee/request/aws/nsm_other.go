//go:build !linux

package aws

import (
	"context"
	"errors"
)

// Attest is a stub on this platform; it always returns an error.
func Attest(context.Context, string, []byte, []byte) ([]byte, error) {
	return nil, errors.New("/dev/nsm is only available inside a Nitro enclave (linux)")
}
