package compare

import "bytes"

// RegInfo — Check 가 필요로 하는 체인 등록 정보의 역할 무관 요약. chain/lookup 의 Registration 에서
// 뽑아 채운다(측정값 해시 · 만료 · 서명자).
type RegInfo struct {
	MeasurementHash []byte
	Expires         int64 // 체인 시계 기준, unix 초
	Signer          string
}

// Check — 체인 등록값이 한쪽에라도 낀 비교. 설계문서 M2 규칙: "체인 등록 정보가 낀 대조는 만료 · 승인
// 서명까지 본다" — 역할과 상관없이(Auditor 든 Vendor 든 등록 직후 자기 조회값을 볼 때든, User 의 최종
// 확인이든) 체인 값이 한쪽에 있으면 항상 이걸 쓴다. 값이 같은지 + 등록이 아직 만료 전인지 + 서명자가
// trusted(보통 컨트랙트의 AUDITOR · VENDOR 주소) 안에 있는지, 셋 다 맞아야 Pass.
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
