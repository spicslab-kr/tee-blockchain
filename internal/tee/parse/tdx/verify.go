package tdx

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	pb "github.com/google/go-tdx-guest/proto/tdx"
	"github.com/google/go-tdx-guest/verify"
)

// Parser: if Now is zero, the certificate chain is judged at the current time. Unlike a Nitro document, a TDX quote
// carries no timestamp, so there is no document time to fall back to. That costs little here: a PCK leaf lives for
// years (the fixture's runs from 2026-10-15 to 2033-10-15), not the three hours of a Nitro leaf, so a quote checked
// some time after it was made is not refused for that reason alone. Freshness comes from the nonce.
type Parser struct {
	Now time.Time
}

// VerifySig runs in this order: decode the quote, check that the PCK certificate carried in the quote chains to the
// root (rootCert) as of Now, then let go-tdx-guest check the quote signature, the Quoting Enclave report signature
// and the hash that binds the attestation key to that report.
//
// Intel's collateral (TCB info, QE identity) and revocation lists are not fetched. They come from Intel's PCS over
// the network, and this check has to run offline, like the Nitro one. So the platform's TCB level is not judged:
// a quote from a machine with an out-of-date TDX module still verifies.
func (a Parser) VerifySig(ctx context.Context, format string, raw []byte, rootCert []byte) error {
	q, err := decode(format, raw)
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
	leaf, inter, err := pckChain(q)
	if err != nil {
		return err
	}
	at := a.Now
	if at.IsZero() {
		at = time.Now()
	}
	// The root at the end of the quote's own chain is never trusted: only the intermediates are taken from the
	// quote, and the root comes from rootCert.
	opts := x509.VerifyOptions{Roots: roots, Intermediates: inter, CurrentTime: at, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}
	if _, err := leaf.Verify(opts); err != nil {
		return fmt.Errorf("%w: %v", ErrChain, err)
	}
	// Collateral and CRLs are not fetched, so only PckCertChain matters; the others are set so no field falls back to
	// the library's own clock.
	now := &verify.TimeSet{PckCertChain: at, TcbInfo: at, QeIdentity: at, PckCrl: at, RootCaCrl: at}
	if err := verify.TdxQuote(q, &verify.Options{Now: now, TrustedRoots: roots}); err != nil {
		return fmt.Errorf("%w: %v", ErrSignature, err)
	}
	return nil
}

// pckChain reads the PEM chain in the quote's certification data: the PCK leaf first, then the PCK Platform (or
// Processor) CA, then the root.
func pckChain(q *pb.QuoteV4) (*x509.Certificate, *x509.CertPool, error) {
	rest := q.GetSignedData().GetCertificationData().GetQeReportCertificationData().GetPckCertificateChainData().GetPckCertChain()
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: PCK chain: %v", ErrBadDocument, err)
		}
		certs = append(certs, c)
	}
	if len(certs) < 2 {
		return nil, nil, fmt.Errorf("%w: PCK chain has %d certificates, want leaf, intermediate and root", ErrBadDocument, len(certs))
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	return certs[0], inter, nil
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
