package aws

import (
	"errors"
	"testing"

	p "tb/internal/tee/parse/Vendor"
	r "tb/internal/tee/request/Vendor"
)

func TestCheckNonce(t *testing.T) {
	doc, nonce := realDoc(t)
	a := Parser{}
	if err := a.CheckNonce(doc, nonce); err != nil {
		t.Fatalf("the nonce we sent must match: %v", err)
	}
	wrong := append(r.Nonce(nil), nonce...)
	wrong[0] ^= 1
	if err := a.CheckNonce(doc, wrong); !errors.Is(err, p.ErrNonce) {
		t.Fatalf("a different nonce must be refused, got %v", err)
	}
	if err := a.CheckNonce(doc, nil); err == nil {
		t.Fatal("checking against no nonce must be an error, not a pass")
	}
}
