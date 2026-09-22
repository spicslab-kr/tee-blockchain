package compare

import "testing"

func TestCompare_AllMatch(t *testing.T) {
	a := Items{{Name: "PCR0", Value: []byte{1}}, {Name: "PCR1", Value: []byte{2}}}
	b := Items{{Name: "PCR0", Value: []byte{1}}, {Name: "PCR1", Value: []byte{2}}}
	v := Compare(a, b)
	if !v.Pass || len(v.Reason) != 0 {
		t.Fatalf("Compare = %+v, want Pass with no reasons", v)
	}
}

func TestCompare_ValueDiffers(t *testing.T) {
	a := Items{{Name: "PCR0", Value: []byte{1}}}
	b := Items{{Name: "PCR0", Value: []byte{2}}}
	v := Compare(a, b)
	if v.Pass {
		t.Fatal("differing values must fail")
	}
	if len(v.Reason) != 1 || v.Reason[0] != "PCR0: 값이 다름" {
		t.Fatalf("Reason = %v", v.Reason)
	}
}

func TestCompare_MissingOnEitherSide(t *testing.T) {
	a := Items{{Name: "PCR0", Value: []byte{1}}}
	b := Items{{Name: "PCR1", Value: []byte{2}}}
	v := Compare(a, b)
	if v.Pass || len(v.Reason) != 2 {
		t.Fatalf("Compare = %+v, want two reasons (missing on each side)", v)
	}
}

func TestCheck_AllGood(t *testing.T) {
	h := []byte{0xaa, 0xbb}
	reg := RegInfo{MeasurementHash: h, Expires: 200, Signer: "0xAbCd"}
	v := Check(h, reg, 100, []string{"0xabcd"}) // trusted 는 소문자, Signer 는 체크섬 — 대소문자 무관하게 맞아야 한다
	if !v.Pass {
		t.Fatalf("Check = %+v, want Pass", v)
	}
}

func TestCheck_Expired(t *testing.T) {
	h := []byte{0xaa}
	reg := RegInfo{MeasurementHash: h, Expires: 100, Signer: "0xabcd"}
	v := Check(h, reg, 100, []string{"0xabcd"}) // now == Expires — 만료로 본다 (딱 그 순간은 더는 유효하지 않음)
	if v.Pass {
		t.Fatal("expires <= now 이면 실패여야 한다")
	}
}

func TestCheck_UntrustedSigner(t *testing.T) {
	h := []byte{0xaa}
	reg := RegInfo{MeasurementHash: h, Expires: 200, Signer: "0xdead"}
	v := Check(h, reg, 100, []string{"0xabcd", "0xbeef"})
	if v.Pass {
		t.Fatal("신뢰 목록에 없는 서명자는 실패여야 한다")
	}
}

func TestCheck_HashMismatch(t *testing.T) {
	reg := RegInfo{MeasurementHash: []byte{0xaa}, Expires: 200, Signer: "0xabcd"}
	v := Check([]byte{0xbb}, reg, 100, []string{"0xabcd"})
	if v.Pass {
		t.Fatal("해시가 다르면 실패여야 한다")
	}
}

func TestCheck_MultipleReasonsAllReported(t *testing.T) {
	reg := RegInfo{MeasurementHash: []byte{0xaa}, Expires: 50, Signer: "0xdead"}
	v := Check([]byte{0xbb}, reg, 100, []string{"0xabcd"})
	if v.Pass || len(v.Reason) != 3 {
		t.Fatalf("만료 · 서명자 · 해시 세 가지 다 틀렸으니 근거 세 개가 다 나와야 한다: %+v", v)
	}
}
