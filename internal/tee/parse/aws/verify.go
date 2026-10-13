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
)

// Parser: if Now is zero, the certificate chain is judged at the document's own time. A Nitro leaf certificate
// lives only about three hours, so a document opened later must be judged by whether it was valid when it was
// created. Freshness is checked separately.
type Parser struct {
	Now time.Time
}

// VerifySig runs in this order: decode the document, check that the leaf certificate chains to the root (rootCert) as of the document time, then check the COSE signature against the leaf certificate key.
func (a Parser) VerifySig(ctx context.Context, format string, raw []byte, rootCert []byte) error {
	cose, doc, err := decode(format, raw)
	if err != nil {
		return err
	}
	if len(rootCert) == 0 {
		rootCert = rootPEM
	}
	roots, err := loadRoots(rootCert)
	if err != nil {
		return err
	}
	leaf, err := x509.ParseCertificate(doc.Certificate)
	if err != nil {
		return fmt.Errorf("%w: leaf certificate: %v", ErrBadDocument, err)
	}
	inter := x509.NewCertPool()
	for _, der := range doc.CABundle {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return fmt.Errorf("%w: cabundle: %v", ErrBadDocument, err)
		}
		inter.AddCert(c)
	}
	at := a.Now
	if at.IsZero() {
		at = time.UnixMilli(int64(doc.Timestamp))
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, CurrentTime: at}); err != nil {
		return fmt.Errorf("%w: %v", ErrChain, err)
	}
	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve.Params().BitSize != 384 {
		return fmt.Errorf("%w: leaf key is not ECDSA P-384", ErrSignature)
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
		return fmt.Errorf("%w: signature is %d bytes, want 96", ErrSignature, len(cose.Signature))
	}
	h := sha512.Sum384(sigStruct)
	rr := new(big.Int).SetBytes(cose.Signature[:48])
	ss := new(big.Int).SetBytes(cose.Signature[48:])
	if !ecdsa.Verify(pub, h[:], rr, ss) {
		return ErrSignature
	}
	return nil
}

// checkAlg requires the protected header to be {1: -35} (ES384).
func checkAlg(protected []byte) error {
	var hdr map[int]int
	if err := cbor.Unmarshal(protected, &hdr); err != nil {
		return fmt.Errorf("%w: protected header: %v", ErrBadDocument, err)
	}
	if hdr[1] != -35 {
		return fmt.Errorf("%w: alg %d, want -35 (ES384)", ErrSignature, hdr[1])
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
