package aws

import (
	"bytes"
	"errors"
)

// CheckNonce — 문서의 nonce 가 내가 보낸 것과 같은지.
func (a Parser) CheckNonce(format string, raw []byte, nonce []byte) error {
	_, doc, err := decode(format, raw)
	if err != nil {
		return err
	}
	if len(nonce) == 0 {
		return errors.New("no nonce to check against")
	}
	if !bytes.Equal(doc.Nonce, nonce) {
		return ErrNonce
	}
	return nil
}
