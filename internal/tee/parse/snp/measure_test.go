package snp

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

// The recording booted /boot/initrd.img-7.0.0-1017-gcp after GRUB_FORCE_PARTUUID was turned off. Every value is checked
// against something read outside this parser: INITRD and KERNEL against sha256sum of the files on the instance,
// CMDLINE against sha256sum of the command line text, MEASUREMENT and PCR4 against the earlier recording of the same
// instance (the firmware and the boot loader did not change).
func TestParse_RealEvidence(t *testing.T) {
	ms, err := (Parser{}).Parse(FormatInitrd, realDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []Measurement{
		{"MEASUREMENT", mustHex(t, "f67900851e25a378f04b4d4b5f1ea4410c11deb74e97ddcc7e3363a33f6e1dbb7880d304005387bc98e9cbdceec4ea28")},
		{"PCR4", mustHex(t, "5b65d2d390d40a5dffb29675a90939615b25aaf164a8b493743242e6f5e84389")},
		// sha256sum /boot/vmlinuz-7.0.0-1017-gcp on the instance
		{"KERNEL", mustHex(t, "3cf8c2c34cd30720d3f37c8b5327ae7d10f24073e73a34edb1e4aaea4dd39adb")},
		// sha256sum of the command line text checked below, computed outside Go
		{"CMDLINE", mustHex(t, "80dfd7d3a1f7ba936075eb6666f5e72c865fa2c11295ba0f35cfbdb276cc3de0")},
		// sha256sum /boot/initrd.img-7.0.0-1017-gcp on the instance
		{"INITRD", mustHex(t, "4703ec07684196e8616d384ac585be54b6345ca8c6ea5544aab8445e0cd4b0a2")},
	}
	if len(ms) != len(want) {
		t.Fatalf("got %d values, want %d: %v", len(ms), len(want), ms)
	}
	for i, w := range want {
		if ms[i].Name != w.Name || !bytes.Equal(ms[i].Value, w.Value) {
			t.Errorf("value %d = %s %x, want %s %x", i, ms[i].Name, ms[i].Value, w.Name, w.Value)
		}
	}

	ev, err := decode(FormatInitrd, realDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bootValues(ev)
	if err != nil {
		t.Fatal(err)
	}
	if b.cmdlineText != "/vmlinuz-7.0.0-1017-gcp root=UUID=9d4d258d-90ac-4d94-b296-007fa0812719 ro console=ttyS0,115200" {
		t.Errorf("command line %q", b.cmdlineText)
	}
	if !isZero(b.pcrs[12]) {
		t.Error("PCR12 is expected to be untouched without the verity hook")
	}
}

// The same evidence has no root hash on its command line and an untouched PCR12, so it is not verity evidence.
func TestParse_VerityNeedsRootHash(t *testing.T) {
	_, err := (Parser{}).Parse(FormatVerity, realDoc(t))
	if !errors.Is(err, ErrBadDocument) || !strings.Contains(err.Error(), verityRootHashArg) {
		t.Fatalf("an initrd boot without a root hash must not pass as verity, got %v", err)
	}
}

// The older recording booted the stock Ubuntu entry, which on GCP is an initrdless boot (GRUB_FORCE_PARTUUID): GRUB
// loaded the kernel and no initrd. So neither format may accept it.
func TestParse_InitrdlessBootIsRefused(t *testing.T) {
	for _, f := range []string{FormatInitrd, FormatVerity} {
		_, err := (Parser{}).Parse(f, initrdlessDoc(t))
		if !errors.Is(err, ErrBadDocument) || !strings.Contains(err.Error(), "no initrd") {
			t.Errorf("%s: an initrdless boot must be refused, got %v", f, err)
		}
	}
}

func TestBootValues_Initrdless(t *testing.T) {
	ev, err := decode(FormatInitrd, initrdlessDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bootValues(ev)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		// vTPM PCR4, SHA-256 bank
		"PCR4": "5b65d2d390d40a5dffb29675a90939615b25aaf164a8b493743242e6f5e84389",
		// the file GRUB measured as /vmlinuz-7.0.0-1017-gcp
		"KERNEL": "3cf8c2c34cd30720d3f37c8b5327ae7d10f24073e73a34edb1e4aaea4dd39adb",
		// sha256sum of the command line text below, computed outside Go
		"CMDLINE": "76d415e0ae88a958465b8ee68ef57dbc4abc733a6faf5946fe56cd25b033b299",
	}
	got := map[string][]byte{"PCR4": b.pcrs[4], "KERNEL": b.kernel, "CMDLINE": b.cmdline}
	for k, v := range want {
		if hex.EncodeToString(got[k]) != v {
			t.Errorf("%s = %x, want %s", k, got[k], v)
		}
	}
	if b.cmdlineText != "/vmlinuz-7.0.0-1017-gcp root=PARTUUID=27c09ec4-48e8-4dfd-be36-678080b236c7 ro console=ttyS0,115200 panic=-1" {
		t.Errorf("command line %q", b.cmdlineText)
	}
	if b.initrd != nil {
		t.Error("no initrd was loaded, so none must be reported")
	}
	if !isZero(b.pcrs[12]) {
		t.Error("PCR12 is expected to be untouched on a stock boot")
	}
	if m := hex.EncodeToString(ev.report.GetReport().GetMeasurement()); m != "f67900851e25a378f04b4d4b5f1ea4410c11deb74e97ddcc7e3363a33f6e1dbb7880d304005387bc98e9cbdceec4ea28" {
		t.Errorf("MEASUREMENT = %s", m)
	}
}

func TestParse_DebugGuestIsRefused(t *testing.T) {
	report := append([]byte(nil), gcpReport...)
	report[0x08+2] |= 0x08 // POLICY bit 19 (DEBUG): byte 2 of the little-endian POLICY at 08h, bit 3
	raw, err := Encode(report, gcpCerts, gcpVTPM)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Parser{}).Parse(FormatInitrd, raw); !errors.Is(err, ErrBadDocument) || !strings.Contains(err.Error(), "DEBUG") {
		t.Fatalf("a debug guest's measurements must be refused, got %v", err)
	}
	info, err := (Parser{}).Info(FormatInitrd, raw)
	if err != nil || !info.Debug {
		t.Fatalf("Info must report debug mode: %+v, %v", info, err)
	}
}

func TestParse_FormatIsRequired(t *testing.T) {
	if _, err := (Parser{}).Parse("", realDoc(t)); !errors.Is(err, ErrWrongFormat) {
		t.Fatalf("without a format the values to register are unknown, got %v", err)
	}
}

// measurementHash covers the sorted name/value list, so the two formats must not share a name set.
func TestMeasurementNames_DifferPerFormat(t *testing.T) {
	i, v := MeasurementNames(FormatInitrd), MeasurementNames(FormatVerity)
	if len(i) != 5 || len(v) != 6 || v[5] != "PCR12" {
		t.Fatalf("initrd %v, verity %v", i, v)
	}
	i[0] = "changed"
	if MeasurementNames(FormatInitrd)[0] != "MEASUREMENT" {
		t.Fatal("MeasurementNames must return a copy")
	}
}

func TestInfo(t *testing.T) {
	info, err := (Parser{}).Info(FormatInitrd, realDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != FormatInitrd || info.Debug || hex.EncodeToString(info.Nonce) != gcpNonce {
		t.Fatalf("info: %+v", info)
	}
	if info.Timestamp != (time.Time{}) || info.Module != "" {
		t.Fatalf("the evidence has no time and no module, got %v and %q", info.Timestamp, info.Module)
	}
}
