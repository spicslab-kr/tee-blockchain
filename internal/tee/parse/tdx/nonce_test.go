package tdx

import (
	"errors"
	"testing"
)

func TestCheckNonce(t *testing.T) {
	raw, nonce := realQuote(t)
	a := Parser{}
	if err := a.CheckNonce(Format, raw, nonce); err != nil {
		t.Fatalf("the nonce we sent must match: %v", err)
	}
	wrong := append([]byte(nil), nonce...)
	wrong[0] ^= 1
	if err := a.CheckNonce(Format, raw, wrong); !errors.Is(err, ErrNonce) {
		t.Fatalf("a different nonce must be refused, got %v", err)
	}
	if err := a.CheckNonce(Format, raw, nil); err == nil {
		t.Fatal("checking against no nonce must be an error, not a pass")
	}
	if err := a.CheckNonce(Format, raw, append(nonce, 0)); !errors.Is(err, ErrNonce) {
		t.Fatalf("a nonce longer than REPORTDATA must be refused, got %v", err)
	}
}

// A 32-byte request nonce occupies the front of REPORTDATA and the rest is zero.
func TestCheckNonce_ShortNonceNeedsZeroTail(t *testing.T) {
	raw, nonce := realQuote(t)
	// The fixture's REPORTDATA is a full 64 bytes, so its first 32 alone must not pass: the tail is not zero.
	if err := (Parser{}).CheckNonce(Format, raw, nonce[:32]); !errors.Is(err, ErrNonce) {
		t.Fatalf("a prefix of REPORTDATA must not pass as the nonce, got %v", err)
	}
	for i := offReportData + 32; i < offReportData+64; i++ {
		raw[i] = 0
	}
	if err := (Parser{}).CheckNonce(Format, raw, nonce[:32]); err != nil {
		t.Fatalf("a 32-byte nonce followed by zeros must pass: %v", err)
	}
}
