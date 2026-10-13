// Package user parses RA documents: it verifies the signature and nonce and extracts only the measurements.
package user

import (
	"context"
	"errors"
	"time"

	r "attestamp/internal/tee/request/user"
)

type Measurement struct {
	Name  string `json:"name"`
	Value []byte `json:"value"` // base64
}

type Measurements []Measurement

// Get finds a value by name, or returns nil.
func (ms Measurements) Get(name string) []byte {
	for _, m := range ms {
		if m.Name == name {
			return m.Value
		}
	}
	return nil
}

// There is one Parser per platform. key is that platform's verification key (e.g. the Nitro root certificate PEM); if empty, the built-in one is used.
type Parser interface {
	// VerifySig checks that the platform signed the document: the signature and the certificate chain.
	VerifySig(ctx context.Context, ra r.RADoc, key []byte) error
	// CheckNonce checks that the nonce inside the document is the one I sent.
	CheckNonce(ra r.RADoc, nonce r.Nonce) error
	// Parse extracts the measurements. It does not check the signature; run VerifySig first.
	Parse(ra r.RADoc) (Measurements, error)
}

// Info is what the document says besides the measurements. Comparison uses it to check expiry, freshness and debug mode.
type Info struct {
	Format    string
	Timestamp time.Time // when the document was created; certificates are judged valid as of this time
	Nonce     []byte
	UserData  []byte
	Debug     bool // debug-mode TEE: the measurements are meaningless
	Module    string
}

var (
	ErrWrongFormat = errors.New("document format does not match this parser")
	ErrBadDocument = errors.New("document is malformed")
	ErrSignature   = errors.New("document signature does not verify")
	ErrChain       = errors.New("document certificate does not chain to the platform root")
	ErrNonce       = errors.New("document nonce is not the one sent")
)
