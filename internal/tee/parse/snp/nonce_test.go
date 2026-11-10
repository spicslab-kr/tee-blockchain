package snp

import (
	"encoding/hex"
	"errors"
	"testing"
)

func TestCheckNonce(t *testing.T) {
	raw := realDoc(t)
	nonce, _ := hex.DecodeString(gcpNonce)
	a := Parser{}
	if err := a.CheckNonce(FormatInitrd, raw, nonce); err != nil {
		t.Fatalf("the nonce we sent must match: %v", err)
	}
	wrong := append([]byte(nil), nonce...)
	wrong[0] ^= 1
	if err := a.CheckNonce(FormatInitrd, raw, wrong); !errors.Is(err, ErrNonce) {
		t.Fatalf("a different nonce must be refused, got %v", err)
	}
	if err := a.CheckNonce(FormatInitrd, raw, nil); err == nil {
		t.Fatal("checking against no nonce must be an error, not a pass")
	}
}
