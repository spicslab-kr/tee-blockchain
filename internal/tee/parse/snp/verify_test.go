package snp

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

func TestVerifySig_RealEvidenceWithEmbeddedRoots(t *testing.T) {
	for _, f := range []string{FormatInitrd, FormatVerity} {
		if err := (Parser{}).VerifySig(context.Background(), f, realDoc(t), nil); err != nil {
			t.Fatalf("%s: genuine evidence must verify: %v", f, err)
		}
	}
}

func TestVerifySig_ChainsAreJudgedAtNow(t *testing.T) {
	// The fixture's AK certificate was issued 2026-11-10 15:33 UTC; the VCEK 2026-11-01.
	err := (Parser{Now: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)}).VerifySig(context.Background(), FormatInitrd, realDoc(t), nil)
	if !errors.Is(err, ErrChain) {
		t.Fatalf("before the AK certificate was issued its chain must not hold: %v", err)
	}
	// The VCEK expires 2033-11-01; after that the AMD side fails.
	if err := (Parser{Now: time.Date(2034, 1, 1, 0, 0, 0, 0, time.UTC)}).VerifySig(context.Background(), FormatInitrd, realDoc(t), nil); err == nil {
		t.Fatal("after the VCEK expires the evidence must not verify")
	}
}

func TestVerifySig_WrongCloudRootIsRefused(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "other root"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	other := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	err := (Parser{}).VerifySig(context.Background(), FormatInitrd, realDoc(t), other)
	if !errors.Is(err, ErrChain) {
		t.Fatalf("an AK certificate must not chain to a root it was not issued under, got %v", err)
	}
}

func TestVerifySig_TamperedReportFailsSignature(t *testing.T) {
	report := append([]byte(nil), gcpReport...)
	report[0x90] ^= 0x01 // first byte of MEASUREMENT
	raw, err := Encode(report, gcpCerts, gcpVTPM)
	if err != nil {
		t.Fatal(err)
	}
	if err := (Parser{}).VerifySig(context.Background(), FormatInitrd, raw, nil); !errors.Is(err, ErrSignature) {
		t.Fatalf("a changed report must fail the AMD signature, got %v", err)
	}
}

// Two documents that are each genuine but not bound to each other must be refused. Appending an unknown protobuf field
// keeps the vTPM attestation valid (the quotes are untouched) but changes its SHA3-512, so REPORT_DATA no longer covers it.
func TestVerifySig_UnboundDocumentsAreRefused(t *testing.T) {
	vtpm := append(append([]byte(nil), gcpVTPM...), 0xF8, 0x07, 0x01) // field 127, varint 1
	raw, err := Encode(gcpReport, gcpCerts, vtpm)
	if err != nil {
		t.Fatal(err)
	}
	err = (Parser{}).VerifySig(context.Background(), FormatInitrd, raw, nil)
	if !errors.Is(err, ErrSignature) || !strings.Contains(err.Error(), "REPORT_DATA") {
		t.Fatalf("a vTPM attestation the report does not cover must be refused, got %v", err)
	}
}

// The older recording is genuine on both sides but bound with SHA-512, the binding used until 2026-11-11. Only the
// binding may fail: the AMD signature and the AK chain still hold.
func TestVerifySig_SHA512BindingIsRefused(t *testing.T) {
	err := (Parser{}).VerifySig(context.Background(), FormatInitrd, initrdlessDoc(t), nil)
	if !errors.Is(err, ErrSignature) || !strings.Contains(err.Error(), "SHA3-512") {
		t.Fatalf("evidence bound with SHA-512 must be refused at the binding, got %v", err)
	}
	ev, err := decode(FormatInitrd, initrdlessDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	old := sha512.Sum512(ev.vtpmRaw)
	if !bytes.Equal(ev.reportData(), old[:]) {
		t.Fatal("the older recording is expected to carry SHA-512 of its vTPM attestation")
	}
}

// REPORT_DATA of the current recording, checked with a SHA3-512 computed outside Go (openssl dgst -sha3-512).
func TestBinding_MatchesOpenSSL(t *testing.T) {
	ev, err := decode(FormatInitrd, realDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "82ded2584e3611a7e4252a051a93a0536053963f98e5256a4cb8ecd822cd2013697025acbc6f1f125fd7c7b5112b809a5ddacc2ac877655f5c5ec78d354b142e")
	if !bytes.Equal(bindingSum(ev.vtpmRaw), want) || !bytes.Equal(ev.reportData(), want) {
		t.Fatalf("binding %x, REPORT_DATA %x", bindingSum(ev.vtpmRaw), ev.reportData())
	}
}

func TestVerifySig_RefusesOtherFormatsAndGarbage(t *testing.T) {
	raw := realDoc(t)
	a := Parser{}
	if _, err := a.Info("aws-nitro", raw); !errors.Is(err, ErrWrongFormat) {
		t.Fatalf("want ErrWrongFormat, got %v", err)
	}
	if _, err := a.Info(FormatInitrd, []byte("not cbor")); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("want ErrBadDocument, got %v", err)
	}
	short, _ := Encode(gcpReport[:1000], gcpCerts, gcpVTPM)
	if err := a.VerifySig(context.Background(), FormatInitrd, short, nil); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("truncated report: want ErrBadDocument, got %v", err)
	}
	extra, _ := cbor.Marshal(map[string][]byte{"report": gcpReport, "certs": gcpCerts, "vtpm": gcpVTPM, "other": {1}})
	if _, err := a.Info(FormatInitrd, extra); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("an unknown container key must be refused, got %v", err)
	}
	noVTPM, _ := Encode(gcpReport, gcpCerts, nil)
	if _, err := a.Info(FormatInitrd, noVTPM); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("missing vTPM attestation: want ErrBadDocument, got %v", err)
	}
}

func TestEncode_IsDeterministic(t *testing.T) {
	a, _ := Encode(gcpReport, gcpCerts, gcpVTPM)
	b, _ := Encode(gcpReport, gcpCerts, gcpVTPM)
	if string(a) != string(b) {
		t.Fatal("the same evidence encoded twice differs")
	}
}
