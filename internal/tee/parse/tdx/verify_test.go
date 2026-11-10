package tdx

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

func TestVerifySig_RealQuoteWithEmbeddedRoot(t *testing.T) {
	raw, _ := realQuote(t)
	if err := (Parser{}).VerifySig(context.Background(), Format, raw, nil); err != nil {
		t.Fatalf("a genuine quote must verify against the Intel SGX root: %v", err)
	}
}

func TestVerifySig_ChainIsJudgedAtNow(t *testing.T) {
	raw, _ := realQuote(t)
	// The fixture's PCK leaf is valid from 2026-10-15 to 2033-10-15.
	err := (Parser{Now: time.Date(2034, 1, 1, 0, 0, 0, 0, time.UTC)}).VerifySig(context.Background(), Format, raw, nil)
	if !errors.Is(err, ErrChain) {
		t.Fatalf("after the PCK leaf expires the chain must lapse: %v", err)
	}
	err = (Parser{Now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}).VerifySig(context.Background(), Format, raw, nil)
	if !errors.Is(err, ErrChain) {
		t.Fatalf("before the PCK leaf was issued the chain must not hold: %v", err)
	}
}

func TestVerifySig_WrongRootIsRefused(t *testing.T) {
	raw, _ := realQuote(t)
	// A root we just created, unrelated to Intel
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "other root"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	other := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	err := (Parser{}).VerifySig(context.Background(), Format, raw, other)
	if !errors.Is(err, ErrChain) {
		t.Fatalf("a quote must not chain to a root it was not issued under, got %v", err)
	}
}

func TestVerifySig_TamperedBodyFailsSignature(t *testing.T) {
	for name, off := range map[string]int{
		"RTMR2":      rtmrOffset(2),
		"MRTD":       offMRTD,
		"REPORTDATA": offReportData,
		"DEBUG bit":  offTDAttributes,
	} {
		raw, _ := realQuote(t)
		raw[off] ^= 0x01 // change one bit of the signed body
		err := (Parser{}).VerifySig(context.Background(), Format, raw, nil)
		if !errors.Is(err, ErrSignature) {
			t.Errorf("%s: a tampered quote must fail the signature, got %v", name, err)
		}
	}
}

func TestVerifySig_RefusesOtherFormatsAndGarbage(t *testing.T) {
	raw, _ := realQuote(t)
	a := Parser{}
	if _, err := a.Parse("aws-nitro", raw); !errors.Is(err, ErrWrongFormat) {
		t.Fatalf("want ErrWrongFormat, got %v", err)
	}
	if _, err := a.Parse(Format, []byte("not a quote")); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("want ErrBadDocument, got %v", err)
	}
	if err := a.VerifySig(context.Background(), Format, raw[:100], nil); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("truncated: want ErrBadDocument, got %v", err)
	}
	if err := a.VerifySig(context.Background(), Format, raw[:1000], nil); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("truncated inside the signature: want ErrBadDocument, got %v", err)
	}
	sev := append([]byte(nil), raw...)
	sev[4] = 0x00 // TEE type 0x00 is SGX, not TDX
	if _, err := a.Parse(Format, sev); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("non-TDX TEE type: want ErrBadDocument, got %v", err)
	}
}
