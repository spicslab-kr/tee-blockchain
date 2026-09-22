package aws

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/fxamacker/cbor/v2"

	p "tb/internal/tee/parse/auditor"
	r "tb/internal/tee/request/auditor"
)

// Parser — Now 가 0 이면 인증서 체인을 문서의 시각에서 판단한다. Nitro 잎 인증서는 세 시간쯤 살아서,
// 나중에 다시 열어 보는 문서는 "만들어졌을 때 유효했나" 로 봐야 한다. 신선도는 따로 본다.
type Parser struct {
	Now time.Time
}

var _ p.Parser = (*Parser)(nil)

// VerifySig — 순서: 문서 해석 → 잎 인증서가 루트(key)까지 이어지는지(문서 시각 기준) → COSE 서명이 잎 인증서 키로 맞는지.
func (a Parser) VerifySig(ctx context.Context, ra r.RADoc, key []byte) error {
	cose, doc, err := decode(ra)
	if err != nil {
		return err
	}
	if len(key) == 0 {
		key = rootPEM
	}
	roots, err := loadRoots(key)
	if err != nil {
		return err
	}
	leaf, err := x509.ParseCertificate(doc.Certificate)
	if err != nil {
		return fmt.Errorf("%w: leaf certificate: %v", p.ErrBadDocument, err)
	}
	inter := x509.NewCertPool()
	for _, der := range doc.CABundle {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return fmt.Errorf("%w: cabundle: %v", p.ErrBadDocument, err)
		}
		inter.AddCert(c)
	}
	at := a.Now
	if at.IsZero() {
		at = time.UnixMilli(int64(doc.Timestamp))
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, CurrentTime: at}); err != nil {
		return fmt.Errorf("%w: %v", p.ErrChain, err)
	}
	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve.Params().BitSize != 384 {
		return fmt.Errorf("%w: leaf key is not ECDSA P-384", p.ErrSignature)
	}
	if err := checkAlg(cose.Protected); err != nil {
		return err
	}
	// Sig_structure = ["Signature1", protected, external_aad, payload]
	sigStruct, err := cbor.Marshal([]any{"Signature1", cose.Protected, []byte{}, cose.Payload})
	if err != nil {
		return err
	}
	if len(cose.Signature) != 96 {
		return fmt.Errorf("%w: signature is %d bytes, want 96", p.ErrSignature, len(cose.Signature))
	}
	h := sha512.Sum384(sigStruct)
	rr := new(big.Int).SetBytes(cose.Signature[:48])
	ss := new(big.Int).SetBytes(cose.Signature[48:])
	if !ecdsa.Verify(pub, h[:], rr, ss) {
		return p.ErrSignature
	}
	return nil
}

// checkAlg — protected header 는 {1: -35} (ES384) 여야 한다.
func checkAlg(protected []byte) error {
	var hdr map[int]int
	if err := cbor.Unmarshal(protected, &hdr); err != nil {
		return fmt.Errorf("%w: protected header: %v", p.ErrBadDocument, err)
	}
	if hdr[1] != -35 {
		return fmt.Errorf("%w: alg %d, want -35 (ES384)", p.ErrSignature, hdr[1])
	}
	return nil
}

func loadRoots(pemBytes []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	n := 0
	for {
		var block *pem.Block
		block, pemBytes = pem.Decode(pemBytes)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("platform root: %w", err)
		}
		pool.AddCert(c)
		n++
	}
	if n == 0 {
		return nil, errors.New("platform root: no CERTIFICATE in PEM")
	}
	return pool, nil
}
