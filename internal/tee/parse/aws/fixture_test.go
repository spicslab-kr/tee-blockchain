package aws

import (
	_ "embed"
	"encoding/hex"
	"strings"
	"testing"
)

// 실제 Nitro 엔클레이브(nginx, my-web)에서 받은 문서와 그때 보낸 난수.
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
