//go:build !linux

package snp

import "errors"

// GCEAttester attests with the GCE vTPM's RSA attestation key. It only works inside a Linux guest.
type GCEAttester struct {
	Device string
}

func (GCEAttester) Attest([]byte) ([]byte, error) {
	return nil, errors.New("the GCE vTPM is only available inside a Linux guest")
}
