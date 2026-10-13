// Package user compares two sets of values and reports match or mismatch with reasons.
//
// The actual logic lives once, role-independent, in internal/core/compare. This package is a delegate that
// converts this role's tee/parse/user.Measurements into the shape that logic takes; see the comment in the
// auditor package's compare.go for the reason.
//
// measurementHash is repeated here because the User never registers, so there is no chain/register/user. The
// computation must be identical to MeasurementHash in chain/register (and to the one in chain/lookup/user/ether).
package user

import (
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"golang.org/x/crypto/sha3"

	"attestamp/internal/core/compare"

	l "attestamp/internal/chain/lookup/user"
	p "attestamp/internal/tee/parse/user"
)

type Verdict = compare.Verdict

type Comparer interface {
	Compare(a, b p.Measurements) Verdict
	Check(a p.Measurements, reg l.Registration, now int64, trusted []string) Verdict
}

type Impl struct{}

var _ Comparer = (*Impl)(nil)

func toItems(m p.Measurements) compare.Items {
	out := make(compare.Items, len(m))
	for i, v := range m {
		out[i] = compare.Item{Name: v.Name, Value: v.Value}
	}
	return out
}

func (Impl) Compare(a, b p.Measurements) Verdict {
	return compare.Compare(toItems(a), toItems(b))
}

func (Impl) Check(a p.Measurements, r l.Registration, now int64, trusted []string) Verdict {
	h, err := measurementHash(a)
	if err != nil {
		return Verdict{Reason: []string{"could not compute the measurement hash: " + err.Error()}}
	}
	return compare.Check(h[:], compare.RegInfo{
		MeasurementHash: r.Values.Get("measurementHash"),
		Expires:         r.Expires,
		Signer:          r.Signer,
	}, now, trusted)
}

// measurementHash is the same computation as MeasurementHash in chain/register: keccak256 of "name=value(hex)\n" lines in name order.
func measurementHash(values p.Measurements) ([32]byte, error) {
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

// Clock returns the time to use as Check's now: the time of the block the registration was read from
// (Registration.ChainTime) if present, otherwise (old lookup results, test stubs) this machine's clock. The
// contract judges expiry by block.timestamp, so the verifier must use the same clock. With no revocation,
// expiry is the only way an approval dies, and a slow clock on the verifier's laptop would make a dead
// approval look alive.
func Clock(r l.Registration) int64 {
	if r.ChainTime > 0 {
		return r.ChainTime
	}
	return time.Now().Unix()
}
