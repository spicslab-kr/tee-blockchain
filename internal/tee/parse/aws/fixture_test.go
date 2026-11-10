package aws

import (
	_ "embed"
	"encoding/hex"
	"strings"
	"testing"
)

// A document received from a real Nitro enclave (nginx, my-web) and the nonce sent with it.
//
//go:embed testdata/fixture-my-web.cbor
var myWebDoc []byte

//go:embed testdata/fixture-my-web.nonce
var myWebNonceHex string

func realDoc(t *testing.T) (raw []byte, nonce []byte) {
	t.Helper()
	nonce, err := hex.DecodeString(strings.TrimSpace(myWebNonceHex))
	if err != nil {
		t.Fatal(err)
	}
	return myWebDoc, nonce
}
