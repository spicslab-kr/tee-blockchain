package tdx

import (
	"bytes"
	"errors"
	"fmt"
)

// reportDataSize is the size of REPORTDATA in the TD report: 64 bytes chosen by the TD when it asks for a quote.
const reportDataSize = 64

// CheckNonce checks that REPORTDATA carries the nonce I sent.
//
// REPORTDATA is 64 bytes and a tee/request nonce is 32, so the nonce fills the front and the rest must be zero. A
// 64-byte nonce, as in the recorded fixture, fills all of it. A non-zero byte after the nonce is refused: REPORTDATA
// would then carry something the verifier did not send, and nothing here knows how to judge it.
func (a Parser) CheckNonce(format string, raw []byte, nonce []byte) error {
	q, err := decode(format, raw)
	if err != nil {
		return err
	}
	if len(nonce) == 0 {
		return errors.New("no nonce to check against")
	}
	if len(nonce) > reportDataSize {
		return fmt.Errorf("%w: nonce is %d bytes, REPORTDATA holds %d", ErrNonce, len(nonce), reportDataSize)
	}
	want := make([]byte, reportDataSize)
	copy(want, nonce)
	if !bytes.Equal(q.GetTdQuoteBody().GetReportData(), want) {
		return ErrNonce
	}
	return nil
}
