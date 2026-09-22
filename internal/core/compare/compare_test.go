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
