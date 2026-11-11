package tdx

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// The application recordings of 2026-11-11 (testdata/README.md): Option 1 and Option 2, each recorded twice on the
// same guest with the application changed by one byte, both with the archive placed before the base initrd.

var tdxAppCells = []string{"opt1-v1", "opt1-v2", "opt2-v1", "opt2-v2"}

func appQuote(t *testing.T, cell string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "gcp-c3-"+cell+"-quote.bin"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAppFixtures_VerifyAndNonce(t *testing.T) {
	for _, cell := range tdxAppCells {
		raw := appQuote(t, cell)
		if err := (Parser{}).VerifySig(context.Background(), Format, raw, nil); err != nil {
			t.Errorf("%s: %v", cell, err)
		}
		nonce := sha512.Sum512([]byte("attestamp-tdx-" + cell))
		if err := (Parser{}).CheckNonce(Format, raw, nonce[:]); err != nil {
			t.Errorf("%s: nonce: %v", cell, err)
		}
	}
}

// Only RTMR2 moves with the application, in both options: the firmware (MRTD), its configuration (RTMR0) and the boot
// loader and kernel (RTMR1) are the same in all four quotes. This is the measured basis for the RTMR2 mapping in
// docs/build/tdx.md, and for intel-tdx carrying both options under one format.
func TestAppFixtures_OnlyRTMR2Changes(t *testing.T) {
	values := map[string]map[string]string{}
	for _, cell := range tdxAppCells {
		ms, err := (Parser{}).Parse(Format, appQuote(t, cell))
		if err != nil {
			t.Fatalf("%s: %v", cell, err)
		}
		values[cell] = map[string]string{}
		for _, m := range ms {
			values[cell][m.Name] = hex.EncodeToString(m.Value)
		}
	}
	fixed := map[string]string{
		"MRTD":  "f21e6b48743fe14c2d090078ca1197f09ae4c99438677f2c79901ef80e2aeefe2f9a7eb6e3eb3b5047c4f1511731e5d3",
		"RTMR0": "8847ec5d4441a3e41856f9de32c21676a788060c2746c565aee58b90a882fd427e9e45487dd992cd87eaf0b2fcdfee68",
		"RTMR1": "6e1a3f8856f3119c3036497985d35f79c1744ad141eed42a7fe75aea0de87ee9437d9c71e7a04bbc3af59d7d7842734a",
	}
	for _, cell := range tdxAppCells {
		for name, want := range fixed {
			if values[cell][name] != want {
				t.Errorf("%s: %s = %s, want %s", cell, name, values[cell][name], want)
			}
		}
	}
	rtmr2 := map[string]bool{}
	for _, cell := range tdxAppCells {
		rtmr2[values[cell]["RTMR2"]] = true
	}
	if len(rtmr2) != 4 {
		t.Fatalf("RTMR2 must differ in every recording (each has a different application or option), got %d distinct", len(rtmr2))
	}
}
