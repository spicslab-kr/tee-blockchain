package tdx

import (
	_ "embed"
	"encoding/hex"
	"strings"
	"testing"
)

// A quote received from a real GCP TDX guest (c3-standard-4) and the REPORTDATA it was asked for. See testdata/README.md.
//
//go:embed testdata/gcp-c3-quote.bin
var gcpQuote []byte

//go:embed testdata/gcp-c3-quote.nonce
var gcpNonceHex string

// Byte offsets in a version 4 quote: a 48-byte header, then the TD quote body. They are the published layout, so the
// tests can change one field without going through the library under test.
const (
	offTDAttributes = 48 + 120
	offMRTD         = 48 + 136
	offMROwner      = 48 + 232
	offRTMR0        = 48 + 328
	offReportData   = 48 + 520
)

func realQuote(t *testing.T) (raw []byte, nonce []byte) {
	t.Helper()
	nonce, err := hex.DecodeString(strings.TrimSpace(gcpNonceHex))
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), gcpQuote...), nonce
}

// rtmrOffset is where RTMR<i> starts.
func rtmrOffset(i int) int { return offRTMR0 + 48*i }
