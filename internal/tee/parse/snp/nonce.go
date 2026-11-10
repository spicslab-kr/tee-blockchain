package snp

import (
	"bytes"
	"errors"
)

// CheckNonce checks that the vTPM quotes carry the nonce I sent (their extraData).
//
// The SNP report does not carry the nonce itself: its REPORT_DATA is SHA-512 of the vTPM attestation, which contains
// the quotes. So once VerifySig has checked that binding, a fresh nonce in the quotes makes the report fresh as well;
// a report replayed from an earlier request would be bound to an attestation with the earlier nonce.
func (a Parser) CheckNonce(format string, raw []byte, nonce []byte) error {
	ev, err := decode(format, raw)
	if err != nil {
		return err
	}
	if len(nonce) == 0 {
		return errors.New("no nonce to check against")
	}
	extra, err := quoteExtraData(ev)
	if err != nil {
		return err
	}
	if !bytes.Equal(extra, nonce) {
		return ErrNonce
	}
	return nil
}
