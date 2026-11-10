package tdx

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func TestParse_RealQuoteRegisters(t *testing.T) {
	raw, _ := realQuote(t)
	ms, err := (Parser{}).Parse(Format, raw)
	if err != nil {
		t.Fatal(err)
	}
	// Read from the quote at the offsets of the published layout, not through go-tdx-guest.
	want := map[string]string{
		"MRTD":  "f21e6b48743fe14c2d090078ca1197f09ae4c99438677f2c79901ef80e2aeefe2f9a7eb6e3eb3b5047c4f1511731e5d3",
		"RTMR0": "8847ec5d4441a3e41856f9de32c21676a788060c2746c565aee58b90a882fd427e9e45487dd992cd87eaf0b2fcdfee68",
		"RTMR1": "8d55c85732cb5056c5826453f3b47de52c43f421f365af30a31aefc3ca027d7bfb5dba41c12b44a0797b07df0da4e9aa",
		"RTMR2": "f44ab17ac6a021b3eb9207a512e7d9e2beab23d0ce8882cb5f5c0f90d236a2d065e70b0528c902a64dceaf9d3e81c8fc",
	}
	for name, hexv := range want {
		if got := hex.EncodeToString(ms.Get(name)); got != hexv {
			t.Errorf("%s = %s, want %s", name, got, hexv)
		}
	}
	if len(ms) != len(want) {
		t.Errorf("want exactly MRTD and RTMR0-2, got %d values", len(ms))
	}
	// MROWNER is set on GCP and identifies who launched the TD, not the image. RTMR3 is extended at run time.
	for _, name := range []string{"MROWNER", "RTMR3", "MRCONFIGID", "MROWNERCONFIG"} {
		if ms.Get(name) != nil {
			t.Errorf("%s must be dropped because it is not an image measurement", name)
		}
	}
	if bytes.Equal(raw[offMROwner:offMROwner+48], make([]byte, 48)) {
		t.Error("the fixture's MROWNER is expected to be non-zero; the exclusion test above would prove nothing")
	}
}

func TestParse_MatchesPublishedLayout(t *testing.T) {
	raw, _ := realQuote(t)
	ms, err := (Parser{}).Parse(Format, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ms.Get("MRTD"), raw[offMRTD:offMRTD+48]) {
		t.Error("MRTD differs from the bytes at its offset")
	}
	for i, name := range []string{"RTMR0", "RTMR1", "RTMR2"} {
		off := rtmrOffset(i)
		if !bytes.Equal(ms.Get(name), raw[off:off+48]) {
			t.Errorf("%s differs from the bytes at its offset", name)
		}
	}
}

func TestParse_DebugTDIsRefused(t *testing.T) {
	raw, _ := realQuote(t)
	if raw[offTDAttributes]&tdAttributesDebug != 0 {
		t.Fatal("the fixture is expected to come from a production TD")
	}
	raw[offTDAttributes] |= tdAttributesDebug
	if _, err := (Parser{}).Parse(Format, raw); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("a debug TD's measurements must be refused, got %v", err)
	}
	info, err := (Parser{}).Info(Format, raw)
	if err != nil || !info.Debug {
		t.Fatalf("Info must report debug mode: %+v, %v", info, err)
	}
}

func TestInfo(t *testing.T) {
	raw, nonce := realQuote(t)
	info, err := (Parser{}).Info(Format, raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != Format || info.Debug || !bytes.Equal(info.Nonce, nonce) {
		t.Fatalf("info: %+v", info)
	}
	if info.Timestamp != (time.Time{}) || info.Module != "" {
		t.Fatalf("a TDX quote has no time and no module, got %v and %q", info.Timestamp, info.Module)
	}
}
