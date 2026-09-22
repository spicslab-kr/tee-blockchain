package aws

import (
	_ "embed"
	"encoding/hex"
	"strings"
	"testing"

	r "tb/internal/tee/request/user"
)

// 실제 Nitro 엔클레이브(nginx, my-web)에서 받은 문서와 그때 보낸 난수.
//
//go:embed fixture-my-web.cbor
var myWebDoc []byte

//go:embed fixture-my-web.nonce
var myWebNonceHex string

func realDoc(t *testing.T) (r.RADoc, r.Nonce) {
	t.Helper()
	nonce, err := hex.DecodeString(strings.TrimSpace(myWebNonceHex))
	if err != nil {
		t.Fatal(err)
	}
	return r.RADoc{Format: Format, Raw: myWebDoc}, nonce
}
