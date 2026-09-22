// Package user 는 M2 대조·판정 — 값 두 개를 받아 일치/불일치와 근거를 낸다.
//
// 실제 로직은 internal/core/compare 에 역할 무관하게 한 벌만 있다. 여기는 이 역할의
// tee/parse/user.Measurements 를 그 로직이 받는 모양으로 바꿔주는 델리게이트다 — 자세한 이유는
// auditor 쪽 compare.go 의 주석 참고.
//
// measurementHash 는 User 가 등록을 안 해서 chain/register/user 가 없어 여기서 다시 적는다 — 계산 방식은
// chain/register 의 MeasurementHash 와 반드시 같아야 한다 (chain/lookup/user/ether 의 것과도 같다).
package user

import (
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"golang.org/x/crypto/sha3"

	"tb/internal/core/compare"

	l "tb/internal/chain/lookup/user"
	p "tb/internal/tee/parse/user"
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
		return Verdict{Reason: []string{"측정값 해시 계산 실패: " + err.Error()}}
	}
	return compare.Check(h[:], compare.RegInfo{
		MeasurementHash: r.Values.Get("measurementHash"),
		Expires:         r.Expires,
		Signer:          r.Signer,
	}, now, trusted)
}

// measurementHash — chain/register 의 MeasurementHash 와 같은 계산(이름순 "이름=값(hex)\n" 을 keccak256).
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

// Now — 지금 시각을 Check 의 now 로 쓸 수 있는 모양(unix 초)으로.
func Now() int64 { return time.Now().Unix() }
