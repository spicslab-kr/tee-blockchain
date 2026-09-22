// Package compare 는 M2 대조·판정의 실제 로직 — 역할과 무관하다. auditor · Vendor · user 세 역할의
// core/compare/<역할> 은 각자의 tee/parse.Measurements 타입을 여기가 받는 모양(Items)으로 바꿔주기만
// 하는 얇은 델리게이트다. 로직을 고칠 땐 여기 한 곳만 고치면 된다.
//
// 설계문서 M2 — compare(a, b): 측정값 항목별 비교. 체인 등록 정보가 낀 대조(Check)는 값 일치뿐 아니라
// 만료 · 승인 서명자(trusted)까지 본다.
package compare

import "bytes"

// Item · Items — 이름 있는 측정값 하나, 그 묶음. 역할별 Measurement/Measurements 와 필드가 같다.
type Item struct {
	Name  string
	Value []byte
}

type Items []Item

func (items Items) Get(name string) []byte {
	for _, it := range items {
		if it.Name == name {
			return it.Value
		}
	}
	return nil
}

// Verdict — 판정 결과와 근거. Reason 이 비어 있으면 Pass 다.
type Verdict struct {
	Pass   bool
	Reason []string
}

// Compare — a, b 를 이름별로 맞대본다. 한쪽에만 있거나 값이 다르면 그 이름을 근거로 남기고 불일치.
func Compare(a, b Items) Verdict {
	av := map[string][]byte{}
	for _, it := range a {
		av[it.Name] = it.Value
	}
	bv := map[string][]byte{}
	for _, it := range b {
		bv[it.Name] = it.Value
	}
	names := map[string]bool{}
	for n := range av {
		names[n] = true
	}
	for n := range bv {
		names[n] = true
	}
	var reasons []string
	for n := range names {
		va, okA := av[n]
		vb, okB := bv[n]
		switch {
		case !okA:
			reasons = append(reasons, n+": a 에 없음")
		case !okB:
			reasons = append(reasons, n+": b 에 없음")
		case !bytes.Equal(va, vb):
			reasons = append(reasons, n+": 값이 다름")
		}
	}
	return Verdict{Pass: len(reasons) == 0, Reason: reasons}
}

// RegInfo — Check 가 필요로 하는 체인 등록 정보의 역할 무관 요약. chain/lookup 의 Registration 에서
// 뽑아 채운다(측정값 해시 · 만료 · 서명자).
type RegInfo struct {
	MeasurementHash []byte
	Expires         int64 // 체인 시계 기준, unix 초
	Signer          string
}

// Check — 측정값의 해시(호출자가 register 와 같은 방식으로 미리 계산해서 준다)가 체인에 등록된 해시와
// 같은지, 등록이 아직 만료 전인지, 서명자가 trusted(보통 컨트랙트의 AUDITOR · VENDOR 주소) 안에 있는지
// 세 가지를 본다. 셋 다 맞아야 Pass.
func Check(measurementHash []byte, reg RegInfo, now int64, trusted []string) Verdict {
	var reasons []string
	if reg.Expires <= now {
		reasons = append(reasons, "등록이 만료됨")
	}
	if !contains(trusted, reg.Signer) {
		reasons = append(reasons, "서명자("+reg.Signer+")가 신뢰 목록에 없음")
	}
	if !bytes.Equal(measurementHash, reg.MeasurementHash) {
		reasons = append(reasons, "측정값 해시가 체인 등록값과 다름")
	}
	return Verdict{Pass: len(reasons) == 0, Reason: reasons}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if equalAddress(v, s) {
			return true
		}
	}
	return false
}

// equalAddress — 이더리움 주소는 대소문자(체크섬)만 다르고 값은 같을 수 있어서, 소문자로 맞춰 비교한다.
func equalAddress(a, b string) bool {
	return toLower(a) == toLower(b)
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
