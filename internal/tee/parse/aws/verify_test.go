package aws

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

	"github.com/fxamacker/cbor/v2"
)

func TestVerifySig_RealDocumentWithEmbeddedRoot(t *testing.T) {
	raw, _ := realDoc(t)
	if err := (Parser{}).VerifySig(context.Background(), Format, raw, nil); err != nil {
		t.Fatalf("a genuine document must verify against the AWS root: %v", err)
	}
}

func TestVerifySig_ChainIsJudgedAtDocumentTime(t *testing.T) {
	raw, _ := realDoc(t)
	// 잎 인증서는 몇 시간짜리라 오늘 기준으로는 만료됐다. 문서 시각 기준(기본)으로는 유효.
	err := (Parser{Now: time.Now().Add(365 * 24 * time.Hour)}).VerifySig(context.Background(), Format, raw, nil)
	if !errors.Is(err, ErrChain) {
		t.Fatalf("a year later the chain must have lapsed: %v", err)
	}
}

func TestVerifySig_WrongRootIsRefused(t *testing.T) {
	raw, _ := realDoc(t)
	// 우리가 방금 만든, AWS 와 무관한 루트
	key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "other root"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	other := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	err := (Parser{}).VerifySig(context.Background(), Format, raw, other)
	if !errors.Is(err, ErrChain) {
		t.Fatalf("a document must not chain to a root it was not issued under, got %v", err)
	}
}

func TestVerifySig_TamperedPayloadFailsSignature(t *testing.T) {
	raw, _ := realDoc(t)
	var cose coseSign1
	if err := cbor.Unmarshal(raw, &cose); err != nil {
		t.Fatal(err)
	}
	var d document
	if err := cbor.Unmarshal(cose.Payload, &d); err != nil {
		t.Fatal(err)
	}
	d.PCRs[0][0] ^= 0xff // 측정값 한 바이트를 바꾼다
	payload, _ := cbor.Marshal(d)
	cose.Payload = payload
	tampered, _ := cbor.Marshal(cose)
	err := (Parser{}).VerifySig(context.Background(), Format, tampered, nil)
	if !errors.Is(err, ErrSignature) {
		t.Fatalf("tampered document must fail the signature, got %v", err)
	}
}

func TestVerifySig_RefusesOtherFormatsAndGarbage(t *testing.T) {
	raw, _ := realDoc(t)
	a := Parser{}
	if _, err := a.Parse("gcp-cs-jwt", raw); !errors.Is(err, ErrWrongFormat) {
		t.Fatalf("want ErrWrongFormat, got %v", err)
	}
	if _, err := a.Parse(Format, []byte("not cbor")); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("want ErrBadDocument, got %v", err)
	}
	if err := a.VerifySig(context.Background(), Format, raw[:100], nil); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("truncated: want ErrBadDocument, got %v", err)
	}
}
