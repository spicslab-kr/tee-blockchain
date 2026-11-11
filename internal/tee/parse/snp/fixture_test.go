package snp

import (
	_ "embed"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Evidence recorded on a real GCP SEV-SNP guest (n2d-standard-2). See testdata/README.md.
//
// gcp-n2d-*: the guest booted with an initrd, REPORT_DATA = SHA3-512 of the vTPM attestation. This is the evidence the
// parser accepts.
var (
	//go:embed testdata/gcp-n2d-report.bin
	gcpReport []byte
	//go:embed testdata/gcp-n2d-auxblob.bin
	gcpCerts []byte
	//go:embed testdata/gcp-n2d-att.pb
	gcpVTPM []byte
)

// gcp-n2d-initrdless-*: the same instance before its boot was changed. GRUB booted with no initrd (GCP's Ubuntu image
// sets GRUB_FORCE_PARTUUID), and REPORT_DATA is SHA-512 of the vTPM attestation, the binding used until 2026-11-11.
// Both make it evidence the parser must refuse, and its event log is signed, so it cannot be made up in a test.
var (
	//go:embed testdata/gcp-n2d-initrdless-report.bin
	gcpInitrdlessReport []byte
	//go:embed testdata/gcp-n2d-initrdless-auxblob.bin
	gcpInitrdlessCerts []byte
	//go:embed testdata/gcp-n2d-initrdless-att.pb
	gcpInitrdlessVTPM []byte
)

// gcpNonce is the vTPM nonce of both recordings: SHA-256 of "attestamp-snp-fixture-0001".
const gcpNonce = "d529879465187801d4b9ffc1c7d0848a2f028e5df9ab46d3eb7a9d5cdb7f925f"

// realDoc packs the recorded evidence the way the Vendor sends it.
func realDoc(t *testing.T) []byte {
	t.Helper()
	raw, err := Encode(gcpReport, gcpCerts, gcpVTPM)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// initrdlessDoc packs the older, initrdless, SHA-512-bound recording.
func initrdlessDoc(t *testing.T) []byte {
	t.Helper()
	raw, err := Encode(gcpInitrdlessReport, gcpInitrdlessCerts, gcpInitrdlessVTPM)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
