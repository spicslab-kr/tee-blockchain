package aws

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hf/nitrite"
)

func TestParse_RealDocumentPCRs(t *testing.T) {
	raw, _ := realDoc(t)
	ms, err := (Parser{}).Parse(Format, raw)
	if err != nil {
		t.Fatal(err)
	}
	// The values nitro-cli build-enclave produced at the time
	want := map[string]string{
		"PCR0": "a0099c1cd2af380adf19601ad09aa49380fc8c3c78e875ac9fe958fc6a836a8d3c30ca5ff5d66c9dc7293cd30ce67fb1",
		"PCR1": "4b4d5b3661b3efc12920900c80e126e4ce783c522de6c02a2a5bf7af3a2b9327b86776f188e4be1c1c404a129dbda493",
		"PCR2": "62f677d0dea159ce42932fa7cc14b885d53d295136f74347a8572ba5f48a36e82485055108bd4f1fa2a3302ab64424cf",
	}
	for name, hexv := range want {
		if got := hex.EncodeToString(ms.Get(name)); got != hexv {
			t.Errorf("%s = %s, want %s", name, got, hexv)
		}
	}
	for _, m := range ms {
		if len(m.Value) != 48 {
			t.Errorf("%s has %d bytes", m.Name, len(m.Value))
		}
	}
	// PCR4 (parent instance id) is non-zero on a real instance but says where it ran, not what ran —
	// it must not enter a comparison against nitro-cli build values, which only have PCR0·1·2.
	if ms.Get("PCR4") != nil {
		t.Error("PCR4 (parent instance id) must be dropped — it is not an image measurement")
	}
	if len(ms) != 3 {
		t.Errorf("want exactly PCR0·1·2, got %d registers", len(ms))
	}
}

func TestInfo(t *testing.T) {
	raw, nonce := realDoc(t)
	info, err := (Parser{}).Info(Format, raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != Format || info.Debug || string(info.Nonce) != string(nonce) || !strings.HasPrefix(info.Module, "i-05acc9d339a979636-enc") {
		t.Fatalf("info: %+v", info)
	}
	if info.Timestamp.Before(time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)) || info.Timestamp.After(time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("timestamp %v", info.Timestamp)
	}
}

// Use hf/nitrite as the reference and compare our interpretation against it. The library is used in tests only.
func TestAgainstNitrite(t *testing.T) {
	raw, _ := realDoc(t)
	info, _ := (Parser{}).Info(Format, raw)
	res, err := nitrite.Verify(raw, nitrite.VerifyOptions{CurrentTime: info.Timestamp})
	if err != nil || !res.SignatureOK {
		t.Fatalf("nitrite: %v", err)
	}
	ms, _ := (Parser{}).Parse(Format, raw)
	for _, m := range ms {
		var idx uint
		if _, err := fmt.Sscanf(m.Name, "PCR%d", &idx); err != nil {
			t.Fatal(err)
		}
		if string(res.Document.PCRs[idx]) != string(m.Value) {
			t.Errorf("%s differs from nitrite", m.Name)
		}
	}
	if string(res.Document.Nonce) != string(info.Nonce) || res.Document.ModuleID != info.Module {
		t.Error("nonce or module id differs from nitrite")
	}
}
