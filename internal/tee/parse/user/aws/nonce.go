package aws

import (
	"bytes"
	"errors"

	p "tb/internal/tee/parse/user"
	r "tb/internal/tee/request/user"
)

// CheckNonce — 문서의 nonce 가 내가 보낸 것과 같은지.
func (a Parser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	_, doc, err := decode(ra)
	if err != nil {
		return err
	}
	if len(nonce) == 0 {
		return errors.New("no nonce to check against")
	}
	if !bytes.Equal(doc.Nonce, nonce) {
		return p.ErrNonce
	}
	return nil
}
