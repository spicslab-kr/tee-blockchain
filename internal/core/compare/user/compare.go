// Package user compares two sets of values and reports match or mismatch with reasons.
//
// The logic lives in internal/core/compare. This package is a delegate that converts this role's
// tee/parse/user.Measurements into compare.Items; see the auditor package's compare.go.
//
// measurementHash is copied here because the User has no chain/register/user package. It must produce the
// same hash as MeasurementHash in chain/register.
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
	Check(a p.Measurements, reg l.Registration, now int64, trustedApprovers []string) Verdict
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

func (Impl) Check(a p.Measurements, r l.Registration, now int64, trustedApprovers []string) Verdict {
	h, err := measurementHash(a)
	if err != nil {
		return Verdict{Reason: []string{"could not compute the measurement hash: " + err.Error()}}
	}
	return compare.Check(h[:], compare.RegInfo{
		MeasurementHash: r.Values.Get("measurementHash"),
		Expires:         r.Expires,
		Approver:        r.Approver,
	}, now, trustedApprovers)
}

// measurementHash computes MeasurementHash from chain/register: keccak256 of "name=value(hex)\n" lines in name order.
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

// Clock returns the time to pass as Check's now: the time of the block the registration was read from
// (Registration.ChainTime), or this machine's clock when ChainTime is unset (test stubs). The registry
// judges expiry by block.timestamp, so the check uses the same clock. Expiry is the only way an approval
// ends, and a slow local clock would keep an expired approval looking valid.
func Clock(r l.Registration) int64 {
	if r.ChainTime > 0 {
		return r.ChainTime
	}
	return time.Now().Unix()
}
