// Package auditor 는 M2 대조·판정 — 값 두 개를 받아 일치/불일치와 근거를 낸다.
package auditor

import (
	"errors"

	l "tb/internal/chain/lookup/auditor"
	p "tb/internal/tee/parse/auditor"
)

type Verdict struct {
	Pass   bool
	Reason []string
}

type Comparer interface {
	Compare(a, b p.Measurements) Verdict
	Check(a p.Measurements, reg l.Registration, now int64, trusted []string) Verdict
}

type Impl struct{}

var _ Comparer = (*Impl)(nil)

func (Impl) Compare(a, b p.Measurements) Verdict {
	return Verdict{Reason: []string{errors.New("core/compare/auditor: Compare not implemented").Error()}}
}

func (Impl) Check(a p.Measurements, reg l.Registration, now int64, trusted []string) Verdict {
	return Verdict{Reason: []string{errors.New("core/compare/auditor: Check not implemented").Error()}}
}
