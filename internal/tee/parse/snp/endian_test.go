package snp

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

// Byte order in the SNP report and the host's certificate table. AMD's SEV-SNP Firmware ABI (pub. 56860, read at
// rev. 1.58) states little endian only for the signature (Table 141: R and S "zero-extended little-endian encoded") and
// the public-key formats; for VERSION, POLICY, SIGNER_INFO and the TCB fields it gives no order. go-sev-guest reads
// them little endian. These tests pin that order against the recorded report, on fields whose reserved bits make the
// wrong order detectable. The certificate table's GUIDs follow RFC 4122 (GHCB spec 56421 rev. 2.04, section 4.1.8.1).
// See docs/build/sev-snp.md, "Byte order".

// POLICY read little endian is 0x30000: bit 16 (SMT allowed) and bit 17, which Table 9 says must be one. Read big endian
// bit 17 would be clear, which the ABI forbids.
func TestEndian_PolicyIsLittleEndian(t *testing.T) {
	field := gcpReport[0x08:0x10]
	if v := binary.LittleEndian.Uint64(field); v != 0x30000 {
		t.Fatalf("POLICY read little endian = %#x, want 0x30000", v)
	}
	if v := binary.BigEndian.Uint64(field); v&(1<<17) != 0 {
		t.Fatalf("read big endian, bit 17 should be clear; got %#x", v)
	}
}

// Setting bit 3 of POLICY's last byte sets bit 59, which must be zero (Table 9, bits 63:26 MBZ). The report must be
// refused as malformed. Read big endian it would be a low, allowed bit and pass.
func TestEndian_PolicyHighReservedBitIsRefused(t *testing.T) {
	report := append([]byte(nil), gcpReport...)
	report[0x0F] |= 0x08
	raw, err := Encode(report, gcpCerts, gcpVTPM)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Parser{}).Info(FormatInitrd, raw)
	if !errors.Is(err, ErrBadDocument) {
		t.Fatalf("POLICY bit 59 must be refused as malformed, got %v", err)
	}
	t.Logf("refused: %v", err)
}

// SIGNER_INFO at 48h is a 32-bit field whose bits 31:5 must be zero (Table 23). 0x04 in its last byte is bit 26 read
// little endian, so the report is malformed. Read big endian it would be SIGNING_KEY = 1 (VLEK), a valid value.
func TestEndian_SignerInfoReservedBitsAreRefused(t *testing.T) {
	report := append([]byte(nil), gcpReport...)
	report[0x4B] = 0x04
	raw, err := Encode(report, gcpCerts, gcpVTPM)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Parser{}).Info(FormatInitrd, raw)
	if !errors.Is(err, ErrBadDocument) {
		t.Fatalf("SIGNER_INFO bit 26 must be refused as malformed, got %v", err)
	}
	t.Logf("refused: %v", err)
}

// R is 72 bytes, little endian, zero-extended from 48: bytes 0x2D0-0x2E7 are zero in a genuine report. A non-zero byte
// there makes R a different, larger number, so the signature must fail.
func TestEndian_SignatureRIsLittleEndianZeroExtended(t *testing.T) {
	for i := 0x2D0; i < 0x2E8; i++ {
		if gcpReport[i] != 0 {
			t.Fatalf("byte %#x of R is %#x; a 48-byte R zero-extended little endian leaves it zero", i, gcpReport[i])
		}
	}
	report := append([]byte(nil), gcpReport...)
	report[0x2D0] = 0x01
	raw, err := Encode(report, gcpCerts, gcpVTPM)
	if err != nil {
		t.Fatal(err)
	}
	err = (Parser{}).VerifySig(context.Background(), FormatInitrd, raw, nil)
	if err == nil {
		t.Fatal("a changed zero-extension byte of R must not verify")
	}
	t.Logf("refused: %v", err)
}

// The certificate table's GUIDs are in RFC 4122 order. Rewriting the VCEK entry's GUID in the mixed-endian EFI order
// (first three groups byte-swapped) must make the VCEK unfindable, so the evidence no longer verifies.
func TestEndian_CertTableGUIDIsRFC4122(t *testing.T) {
	vcek := []byte{0x63, 0xda, 0x75, 0x8d, 0xe6, 0x64, 0x45, 0x64}
	certs := append([]byte(nil), gcpCerts...)
	entry := -1
	for off := 0; off+24 <= len(certs); off += 24 {
		if string(certs[off:off+8]) == string(vcek) {
			entry = off
			break
		}
	}
	if entry < 0 {
		t.Fatal("the VCEK GUID 63da758d-e664-4564-... was not found in RFC 4122 order in the certificate table")
	}
	g := certs[entry : entry+8]
	g[0], g[1], g[2], g[3] = g[3], g[2], g[1], g[0]
	g[4], g[5] = g[5], g[4]
	g[6], g[7] = g[7], g[6]
	raw, err := Encode(gcpReport, certs, gcpVTPM)
	if err != nil {
		t.Fatal(err)
	}
	err = (Parser{}).VerifySig(context.Background(), FormatInitrd, raw, nil)
	if !errors.Is(err, ErrSignature) {
		t.Fatalf("with the VCEK GUID in EFI order the VCEK must not be found, got %v", err)
	}
	t.Logf("refused: %v", err)
}
