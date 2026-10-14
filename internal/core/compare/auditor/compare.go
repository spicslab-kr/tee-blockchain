// Package auditor compares two sets of values and reports match or mismatch with reasons.
//
// The logic (per-item comparison, and the trust decision on expiry and trustedApprovers) lives in
// internal/core/compare. This package is a thin delegate that converts this role's tee/parse/auditor.Measurements
// into compare.Items. Each role has its own copy because the Comparer interface takes the role's own
// Measurements type.
package auditor

import (
	"time"

	"attestamp/internal/core/compare"

	l "attestamp/internal/chain/lookup/auditor"
	reg "attestamp/internal/chain/register/auditor"
	p "attestamp/internal/tee/parse/auditor"
)

type Verdict = compare.Verdict

type Comparer interface {
	// Compare matches two sets of measurements, e.g. parsed values vs build values.
	Compare(a, b p.Measurements) Verdict
	// Check is the trust decision against a registration. It passes when the measurement matches, the
	// registration has not expired at now, and the approver is in trustedApprovers.
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
	h, err := reg.MeasurementHash(a)
	if err != nil {
		return Verdict{Reason: []string{"could not compute the measurement hash: " + err.Error()}}
	}
	return compare.Check(h[:], compare.RegInfo{
		MeasurementHash: r.Values.Get("measurementHash"),
		Expires:         r.Expires,
		Approver:        r.Approver,
	}, now, trustedApprovers)
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
