// Package auditor compares two sets of values and reports match or mismatch with reasons.
//
// The actual logic (per-item comparison, expiry and trusted-signer checks) lives once, role-independent, in
// internal/core/compare. This package is a thin delegate that converts this role's tee/parse/auditor.Measurements
// into the shape that logic takes (compare.Items). The same delegate exists for auditor, Vendor and user,
// because the Comparer interface takes a Measurements type and Go cannot share one interface across the
// three per-role types.
package auditor

import (
	"time"

	"tb/internal/core/compare"

	l "tb/internal/chain/lookup/auditor"
	reg "tb/internal/chain/register/auditor"
	p "tb/internal/tee/parse/auditor"
)

type Verdict = compare.Verdict

type Comparer interface {
	// Compare matches two sets of measurements directly, e.g. parsed vs build values, or lookup vs registered values.
	Compare(a, b p.Measurements) Verdict
	// Check compares against an on-chain registration. Besides equal values it checks expiry and the approving signer (trusted).
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
	h, err := reg.MeasurementHash(a)
	if err != nil {
		return Verdict{Reason: []string{"could not compute the measurement hash: " + err.Error()}}
	}
	return compare.Check(h[:], compare.RegInfo{
		MeasurementHash: r.Values.Get("measurementHash"),
		Expires:         r.Expires,
		Signer:          r.Signer,
	}, now, trusted)
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
