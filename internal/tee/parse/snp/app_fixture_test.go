package snp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The application recordings of 2026-11-11 (testdata/README.md): three pairs from the same guest, each pair differing
// only in one byte of the application.
//
//	opt1   Option 1, archive appended without padding: the kernel discarded it (see the *-boot.txt files)
//	opt1p  Option 1, archive appended after the base zero-padded to 4 bytes: unpacked
//	opt2   Option 2, squashfs + dm-verity image on disk, verifying initrd, PCR12

// appDoc packs one application recording, e.g. appDoc(t, "opt1p-v1").
func appDoc(t *testing.T, cell string) []byte {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata", "gcp-n2d-"+cell+"-"+name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	raw, err := Encode(read("report.bin"), read("auxblob.bin"), read("att.pb"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func appFormat(cell string) string {
	if strings.HasPrefix(cell, "opt2") {
		return FormatVerity
	}
	return FormatInitrd
}

var appCells = []string{"opt1-v1", "opt1-v2", "opt1p-v1", "opt1p-v2", "opt2-v1", "opt2-v2"}

func TestAppFixtures_VerifyAndNonce(t *testing.T) {
	for _, cell := range appCells {
		raw := appDoc(t, cell)
		if err := (Parser{}).VerifySig(context.Background(), appFormat(cell), raw, nil); err != nil {
			t.Errorf("%s: %v", cell, err)
		}
		nonce := sha256.Sum256([]byte("attestamp-snp-" + cell))
		if err := (Parser{}).CheckNonce(appFormat(cell), raw, nonce[:]); err != nil {
			t.Errorf("%s: nonce: %v", cell, err)
		}
	}
}

// Two genuine recordings from the same guest, minutes apart, mixed: the report of one with the vTPM attestation of the
// other. Each half verifies on its own (the AMD signature, the AK chain and the quotes are all genuine), so only the
// SHA3-512 binding can catch it: REPORT_DATA of A is the hash of A's attestation, not B's.
//
// This is NOT test-matrix row T10. Both halves come from one VM, and each report was requested over its own
// attestation, so the binding fails by construction. T10 is the relay: a vTPM attestation from VM X, whose hash is
// written into a report requested on another VM Y. There the binding holds, and the question is whether anything else
// refuses it; the 2026-11 record says nothing does. It needs recordings from two VMs (docs/build/app-fixture-recording.md,
// "Reproducing T10"); this test cannot stand in for it.
func TestVerifySig_MixedRecordingsFailTheBinding(t *testing.T) {
	read := func(cell, name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata", "gcp-n2d-"+cell+"-"+name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, pair := range [][2]string{{"opt1p-v1", "opt1p-v2"}, {"opt2-v1", "opt2-v2"}, {"opt1p-v1", "opt2-v1"}} {
		a, b := pair[0], pair[1]
		raw, err := Encode(read(a, "report.bin"), read(a, "auxblob.bin"), read(b, "att.pb"))
		if err != nil {
			t.Fatal(err)
		}
		err = (Parser{}).VerifySig(context.Background(), FormatInitrd, raw, nil)
		if !errors.Is(err, ErrSignature) || !strings.Contains(err.Error(), "REPORT_DATA is not SHA3-512") {
			t.Errorf("report of %s with the vTPM attestation of %s must fail the binding, got %v", a, b, err)
		}
	}
}

func parseApp(t *testing.T, cell string) map[string]string {
	t.Helper()
	ms, err := (Parser{}).Parse(appFormat(cell), appDoc(t, cell))
	if err != nil {
		t.Fatalf("%s: %v", cell, err)
	}
	out := map[string]string{}
	for _, m := range ms {
		out[m.Name] = hex.EncodeToString(m.Value)
	}
	return out
}

// The firmware, boot loader, kernel and command line are those of the distribution-initrd recording; only the initrd
// differs from it.
var bootChain = map[string]string{
	"MEASUREMENT": "f67900851e25a378f04b4d4b5f1ea4410c11deb74e97ddcc7e3363a33f6e1dbb7880d304005387bc98e9cbdceec4ea28",
	"PCR4":        "5b65d2d390d40a5dffb29675a90939615b25aaf164a8b493743242e6f5e84389",
	"KERNEL":      "3cf8c2c34cd30720d3f37c8b5327ae7d10f24073e73a34edb1e4aaea4dd39adb",
}

// Option 1: only INITRD changes with the application, and each INITRD equals the sha256 of the combined initrd that
// mkappinitrd printed on the instance. That holds for the unpadded pair too, although its application never ran: the
// measurement cannot show that (testdata/README.md, "The two Option 1 pairs, side by side").
func TestAppFixtures_Option1ChangesOnlyINITRD(t *testing.T) {
	combined := map[string]string{
		"opt1-v1":  "bc5662827304f527b6e6988e0e2fab248b4a5c28d27665f7666ba87717ef1153",
		"opt1-v2":  "1e3d844a067e5f3fae0dfd3e57c94a0ea08683d759b4e8ff758bde508f68d511",
		"opt1p-v1": "a4504052832e43d87ccd5d1c9ee8e6560be7a2cd528be4a842e8d2c246f358bf",
		"opt1p-v2": "332163d0c30f86c2e81ad48d7df15ab5af5a98eab286d3d8e06cce6b83cb6006",
	}
	for _, pair := range []string{"opt1", "opt1p"} {
		v1, v2 := parseApp(t, pair+"-v1"), parseApp(t, pair+"-v2")
		if len(v1) != 5 || len(v2) != 5 {
			t.Fatalf("%s: want the five amd-sev-snp-initrd values: %v %v", pair, v1, v2)
		}
		for name, want := range bootChain {
			if v1[name] != want || v2[name] != want {
				t.Errorf("%s: %s = %s / %s, want %s in both", pair, name, v1[name], v2[name], want)
			}
		}
		if v1["CMDLINE"] != "80dfd7d3a1f7ba936075eb6666f5e72c865fa2c11295ba0f35cfbdb276cc3de0" || v1["CMDLINE"] != v2["CMDLINE"] {
			t.Errorf("%s: CMDLINE must be the stock one in both: %s / %s", pair, v1["CMDLINE"], v2["CMDLINE"])
		}
		if v1["INITRD"] == v2["INITRD"] {
			t.Errorf("%s: INITRD must change with the application", pair)
		}
		if v1["INITRD"] != combined[pair+"-v1"] || v2["INITRD"] != combined[pair+"-v2"] {
			t.Errorf("%s: INITRD %s / %s, want the combined file's sha256", pair, v1["INITRD"], v2["INITRD"])
		}
	}
}

// Option 2: the verifying initrd is the same file, so INITRD is equal; the root hash on the command line and the PCR12
// the verifying script extends change with the image. PCR12 is recomputed here from the root hash the event log's
// command line carries: SHA-256(32 zero bytes || SHA-256("attestamp-verity-ok:" || root hash)), the formula in
// testdata/record/initramfs-local-bottom.sh.
func TestAppFixtures_Option2ChangesCMDLINEAndPCR12(t *testing.T) {
	v1, v2 := parseApp(t, "opt2-v1"), parseApp(t, "opt2-v2")
	if len(v1) != 6 || len(v2) != 6 {
		t.Fatalf("want the six amd-sev-snp-verity values: %v %v", v1, v2)
	}
	for name, want := range bootChain {
		if v1[name] != want || v2[name] != want {
			t.Errorf("%s = %s / %s, want %s in both", name, v1[name], v2[name], want)
		}
	}
	const verifyingInitrd = "b0fd172dff60ce1ba307d0318ae92d49d2dad8296ae7de8c912e9ab79dabf33a" // sha256sum on the instance
	if v1["INITRD"] != verifyingInitrd || v2["INITRD"] != verifyingInitrd {
		t.Errorf("INITRD = %s / %s, want the verifying initrd %s in both", v1["INITRD"], v2["INITRD"], verifyingInitrd)
	}
	if v1["CMDLINE"] != "2c2f65293fceae9eed803d5fd46996ad8f1d20b945c252c266873d3a0b0b8b52" {
		t.Errorf("CMDLINE v1 = %s, want the sha256 of the logged command line", v1["CMDLINE"])
	}
	if v1["CMDLINE"] == v2["CMDLINE"] || v1["PCR12"] == v2["PCR12"] {
		t.Error("CMDLINE and PCR12 must change with the image")
	}
	for _, cell := range []string{"opt2-v1", "opt2-v2"} {
		ev, err := decode(FormatVerity, appDoc(t, cell))
		if err != nil {
			t.Fatal(err)
		}
		b, err := bootValues(ev)
		if err != nil {
			t.Fatal(err)
		}
		rh := ""
		for _, f := range strings.Fields(b.cmdlineText) {
			if strings.HasPrefix(f, verityRootHashArg) {
				rh = strings.TrimPrefix(f, verityRootHashArg)
			}
		}
		d := sha256.Sum256([]byte("attestamp-verity-ok:" + rh))
		want := sha256.Sum256(append(make([]byte, 32), d[:]...))
		if !bytes.Equal(b.pcrs[12], want[:]) {
			t.Errorf("%s: PCR12 = %x, want %x for root hash %s", cell, b.pcrs[12], want, rh)
		}
	}
}
