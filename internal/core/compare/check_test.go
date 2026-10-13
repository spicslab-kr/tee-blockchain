package compare

import "testing"

func TestCheck_AllGood(t *testing.T) {
	h := []byte{0xaa, 0xbb}
	reg := RegInfo{MeasurementHash: h, Expires: 200, Approver: "0xAbCd"}
	v := Check(h, reg, 100, []string{"0xabcd"}) // trustedApprovers is lower case, Approver is checksummed; case must not matter
	if !v.Pass {
		t.Fatalf("Check = %+v, want Pass", v)
	}
}

func TestCheck_Expired(t *testing.T) {
	h := []byte{0xaa}
	reg := RegInfo{MeasurementHash: h, Expires: 100, Approver: "0xabcd"}
	v := Check(h, reg, 100, []string{"0xabcd"}) // now == Expires counts as expired (not valid at that exact moment)
	if v.Pass {
		t.Fatal("expires <= now must fail")
	}
}

func TestCheck_UntrustedApprover(t *testing.T) {
	h := []byte{0xaa}
	reg := RegInfo{MeasurementHash: h, Expires: 200, Approver: "0xdead"}
	v := Check(h, reg, 100, []string{"0xabcd", "0xbeef"})
	if v.Pass {
		t.Fatal("a approver outside the trusted list must fail")
	}
}

func TestCheck_HashMismatch(t *testing.T) {
	reg := RegInfo{MeasurementHash: []byte{0xaa}, Expires: 200, Approver: "0xabcd"}
	v := Check([]byte{0xbb}, reg, 100, []string{"0xabcd"})
	if v.Pass {
		t.Fatal("a different hash must fail")
	}
}

func TestCheck_MultipleReasonsAllReported(t *testing.T) {
	reg := RegInfo{MeasurementHash: []byte{0xaa}, Expires: 50, Approver: "0xdead"}
	v := Check([]byte{0xbb}, reg, 100, []string{"0xabcd"})
	if v.Pass || len(v.Reason) != 3 {
		t.Fatalf("expiry, approver and hash are all wrong, so all three reasons must be reported: %+v", v)
	}
}
