// Package vendor 는 M2 대조·판정 — 값 두 개를 받아 일치/불일치와 근거를 낸다.
//
// 실제 로직(항목별 대조, 만료 · 신뢰 서명자 확인)은 internal/core/compare 에 역할 무관하게 한 벌만 있다.
// 여기는 이 역할의 tee/parse/auditor.Measurements 를 그 로직이 받는 모양(compare.Items)으로 바꿔주는
// 얇은 델리게이트다. auditor · Vendor · user 세 곳에 똑같이 있다 — Comparer 인터페이스 자체가
// Measurements 타입을 받아서, Go 에서는 역할마다 다른 타입이라 인터페이스를 셋이 같이 쓸 수 없다.
package vendor

import (
	"time"

	"tb/internal/core/compare"

	l "tb/internal/chain/lookup/Vendor"
	reg "tb/internal/chain/register/Vendor"
	p "tb/internal/tee/parse/Vendor"
)

type Verdict = compare.Verdict

type Comparer interface {
	// Compare — 파싱값 vs 빌드값, 조회값 vs 등록값처럼 측정값 묶음 두 개를 그대로 맞대본다.
	Compare(a, b p.Measurements) Verdict
	// Check — 체인 등록 정보가 낀 대조. 값이 같은지뿐 아니라 만료 · 승인 서명자(trusted)까지 본다.
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

// Now — 지금 시각을 Check 의 now 로 쓸 수 있는 모양(unix 초)으로.
func Now() int64 { return time.Now().Unix() }
