// Package vendor requests RA documents: it sends a nonce and receives the RA document.
package vendor

import (
	"context"
	"errors"
	"fmt"
)

// RADoc is the original RA document the TEE returned. It is not opened here.
// Format names the document format and so the parser that opens it: aws-nitro (fully supported), gcp-cs-jwt or azure-maa-jwt (request side only).
type RADoc struct {
	Format string
	Raw    []byte
}

type Requester interface {
	Request(ctx context.Context, nonce Nonce, teeAddr string) (RADoc, error)
}

// Extractor runs inside the TEE: given a nonce, it fetches that TEE's RA document as is.
type Extractor interface {
	Extract(ctx context.Context, nonce Nonce) (RADoc, error)
}

var (
	ErrBadRequest    = errors.New("malformed request")
	ErrBadResponse   = errors.New("malformed response")
	ErrTooLarge      = errors.New("message too large")
	ErrExtractFailed = errors.New("TEE could not produce an attestation document")
	ErrWrongFormat   = errors.New("attestation document has a different format than expected")
)

// ExpectFormat checks that the document format is want. If want is empty, any format is accepted.
func (d RADoc) ExpectFormat(want string) error {
	if want != "" && d.Format != want {
		return fmt.Errorf("%w: got %q, want %q", ErrWrongFormat, d.Format, want)
	}
	return nil
}
