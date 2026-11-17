//go:build !linux

package aws

import (
	"context"
	"errors"
)

// Attest is a stub on this platform; it always returns an error. The role adapters check the nonce's size first, so
// here a wrong-size nonce is reported before this error; only linux requests attestation, so the order does not matter.
func Attest(context.Context, string, []byte, []byte) ([]byte, error) {
	return nil, errors.New("/dev/nsm is only available inside a Nitro enclave (linux)")
}
