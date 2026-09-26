package compare

import "bytes"

// Compare — 체인이 안 낀 비교. a, b 를 이름별로 맞대본다. 한쪽에만 있거나 값이 다르면 그 이름을 근거로
// 남기고 불일치. 설계문서 M2 — 파싱값 vs 빌드값, 파싱값 vs 측정값 기록(또는 다른 TEE 값)이 여기.
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
			reasons = append(reasons, n+": missing in a")
		case !okB:
			reasons = append(reasons, n+": missing in b")
		case !bytes.Equal(va, vb):
			reasons = append(reasons, n+": values differ")
		}
	}
	return Verdict{Pass: len(reasons) == 0, Reason: reasons}
}
