package tdx

import (
	"encoding/binary"
	"testing"
)

// Byte order in a version 4 quote. Intel's "TDX DCAP Quote Generation Library and Quote Verification Library"
// (Appendix A.3) states "Endianess: Little Endian (applies to all integer fields)". TDATTRIBUTES is typed there as a
// byte array; A.3.4 numbers its bits (0 DEBUG, 28 SEPT_VE_DISABLE, 62:32 reserved) but does not say which byte holds
// bit 0. These tests pin the order our code and go-tdx-guest use against the recorded quote, independently of the
// library, so a switch to the other order fails here. See docs/build/tdx.md, "Byte order".

// The header integers read little endian give the values the spec requires: version 4, attestation key type 2
// (ECDSA P-256), TEE type 0x81 (TDX). Read big endian they would be 0x0400, 0x0200 and 0x81000000.
func TestEndian_HeaderIsLittleEndian(t *testing.T) {
	raw, _ := realQuote(t)
	if v := binary.LittleEndian.Uint16(raw[0:2]); v != 4 {
		t.Errorf("version = %d, want 4", v)
	}
	if v := binary.LittleEndian.Uint16(raw[2:4]); v != 2 {
		t.Errorf("attestation key type = %d, want 2", v)
	}
	if v := binary.LittleEndian.Uint32(raw[4:8]); v != 0x81 {
		t.Errorf("TEE type = %#x, want 0x81", v)
	}
}

// TDATTRIBUTES read as a little-endian 64-bit value has exactly bit 28 (SEPT_VE_DISABLE) set, and no reserved bit.
// Read big endian, the same bytes would set bit 36, which A.3.4 reserves. So bit 0 (DEBUG) is in the first byte, which
// is what measure.go's attr[0] check assumes.
func TestEndian_TDAttributesBitNumbering(t *testing.T) {
	raw, _ := realQuote(t)
	field := raw[offTDAttributes : offTDAttributes+8]
	if v := binary.LittleEndian.Uint64(field); v != 1<<28 {
		t.Fatalf("TDATTRIBUTES read little endian = %#x, want only bit 28 (SEPT_VE_DISABLE)", v)
	}
	if v := binary.BigEndian.Uint64(field); v&(0x7fffffff<<32) == 0 {
		t.Fatalf("read big endian the fixture should set a reserved bit (62:32), got %#x", v)
	}
}

// A bit in the last byte of TDATTRIBUTES is bit 56 read little endian, but would be bit 0 (DEBUG) read big endian. It
// must not be taken for DEBUG.
func TestEndian_DebugBitIsNotInLastByte(t *testing.T) {
	raw, _ := realQuote(t)
	raw[offTDAttributes+7] |= 0x01
	info, err := (Parser{}).Info(Format, raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Debug {
		t.Fatal("bit 0 of the last TDATTRIBUTES byte is bit 56, not DEBUG")
	}
	if _, err := (Parser{}).Parse(Format, raw); err != nil {
		t.Fatalf("Parse must not refuse a TD for bit 56: %v", err)
	}
}
