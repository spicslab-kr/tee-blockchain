// Package vendor signs registrations and records them in a Registry contract.
package vendor

import (
	"context"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	l "attestamp/internal/chain/lookup/Vendor"
	p "attestamp/internal/tee/parse/Vendor"

	"golang.org/x/crypto/sha3"
)

// Meta describes a registration. Expires is when it stops counting, on the chain's clock. Registrations are never
// revoked or overwritten.
type Meta struct {
	l.Meta
	Expires time.Time
}

// Signed is a value that has been signed and is ready to send to the chain.
type Signed struct {
	Values    p.Measurements
	Meta      Meta
	Approver  string // address of the signing key; the contract recovers the same address from the signature
	Digest    []byte // the EIP-712 hash that was signed; equals the contract's digest()
	Signature []byte // 65 bytes, r‖s‖v
}

// Receipt confirms a mined registration. RecordKey locates it in the registry.
type Receipt struct {
	RecordKey string // 0x…, 32 bytes
	TxHash string
	Block  uint64
}

// Registrar signs registrations and sends them to one registry.
type Registrar interface {
	// Sign signs the registration with the signing key. The address recovered from the signature is the approver.
	Sign(values p.Measurements, meta Meta, signingKey []byte) (Signed, error)
	// Register sends the signed registration to the contract, which checks the approver and stores it.
	Register(ctx context.Context, s Signed) (Receipt, error)
}

var (
	ErrNotAnApprover     = errors.New("the signer is not the approver fixed in the registry")
	ErrAlreadyRegistered = errors.New("this measurement is already registered on this platform")
	ErrExpiryPassed      = errors.New("expiry is not in the future by the chain's clock")
)

// MeasurementHash reduces a set of measurements to the 32-byte value the contract stores as `measurement`.
// It is keccak256 over "name=value(hex)\n" entries in name order.
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

// Validate checks that every field the contract requires is set.
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
