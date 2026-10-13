// Package auditor registers measurements on chain: it signs them and records them in the contract.
package auditor

import (
	"context"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	l "attestamp/internal/chain/lookup/auditor"
	p "attestamp/internal/tee/parse/auditor"

	"golang.org/x/crypto/sha3"
)

// Meta is the registration info attached to the measurements. Once Expires has passed the registration no longer
// counts. There is no revocation.
type Meta struct {
	l.Meta
	Expires time.Time
}

// Signed is a value that has been signed and is ready to send to the chain.
type Signed struct {
	Values    p.Measurements
	Meta      Meta
	Signer    string // address of the signing key; equals the value the contract recovers from the signature
	Digest    []byte // the hash that was signed; must equal the contract's digest()
	Signature []byte // 65 bytes, r‖s‖v
}

// Receipt confirms a registration. Key is the lookup coordinate.
type Receipt struct {
	Key    string // 0x…, 32 bytes
	TxHash string
	Block  uint64
}

type Registrar interface {
	// Sign signs the measurements and registration info with the signing key. This signature is what says who registered.
	Sign(values p.Measurements, meta Meta, key []byte) (Signed, error)
	// Register sends the signed value to the contract, which checks the signer and then stores it.
	Register(ctx context.Context, s Signed) (Receipt, error)
}

var (
	ErrNotAnApprover     = errors.New("signer is not the approver fixed in the contract")
	ErrAlreadyRegistered = errors.New("this signer already registered this measurement")
	ErrExpiryPassed      = errors.New("expiry is not in the future by the chain's clock")
)

// MeasurementHash reduces a set of measurements to a single 32-byte value, which is what goes into the contract.
// It is keccak256 over "name=value(hex)\n" entries concatenated in name order, so anyone computing it from the same measurements gets the same result.
func MeasurementHash(values p.Measurements) ([32]byte, error) {
	var out [32]byte
	if len(values) == 0 {
		return out, errors.New("no measurements")
	}
	sorted := make(p.Measurements, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha3.NewLegacyKeccak256()
	for i, m := range sorted {
		if m.Name == "" || len(m.Value) == 0 {
			return out, errors.New("measurement with empty name or value")
		}
		if i > 0 && sorted[i-1].Name == m.Name {
			return out, errors.New("duplicate measurement name: " + m.Name)
		}
		h.Write([]byte(m.Name))
		h.Write([]byte("="))
		h.Write([]byte(hex.EncodeToString(m.Value)))
		h.Write([]byte("\n"))
	}
	copy(out[:], h.Sum(nil))
	return out, nil
}

func (m Meta) Validate() error {
	switch {
	case m.Platform == "":
		return errors.New("meta: platform is empty")
	case m.Arch == "":
		return errors.New("meta: arch is empty")
	case m.Service == "":
		return errors.New("meta: service is empty")
	case m.Version == "":
		return errors.New("meta: version is empty")
	case m.Expires.IsZero():
		return errors.New("meta: expires is empty")
	}
	return nil
}
