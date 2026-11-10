package snp

import (
	"context"
	"crypto"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	spb "github.com/google/go-sev-guest/proto/sevsnp"
	"github.com/google/go-sev-guest/verify"
	attest "github.com/google/go-tpm-tools/proto/attest"
	"github.com/google/go-tpm-tools/server"
	"github.com/google/go-tpm/legacy/tpm2"
	"google.golang.org/protobuf/proto"
)

var oidSubjectAltName = asn1.ObjectIdentifier{2, 5, 29, 17}

// Parser: if Now is zero, both certificate chains are judged at the current time. Neither document carries a time: the
// SNP report has no timestamp field and the TPM quote's clock is a counter that starts at an undefined point (see
// docs/build/sev-snp.md, "Time and freshness"). The certificates live long enough for that not to matter in practice
// (a VCEK seven years, the GCE AK certificate thirty), and freshness comes from the nonce.
type Parser struct {
	Now time.Time
}

// VerifySig runs in this order: decode both documents; check the AMD side (VCEK to the ARK go-sev-guest embeds, then
// the report signature); check that the AK certificate chains to the cloud root (rootCert) as of Now; check the quote
// signatures and the event log against the AK; check that REPORT_DATA is SHA-512 of the vTPM attestation.
//
// The AMD certificates come only from the evidence (the host's certificate table); AMD's KDS is never asked, so the
// check runs offline like the Nitro one. CRLs are not checked.
func (a Parser) VerifySig(ctx context.Context, format string, raw []byte, rootCert []byte) error {
	ev, err := decode(format, raw)
	if err != nil {
		return err
	}
	at := a.Now
	if at.IsZero() {
		at = time.Now()
	}

	// go-sev-guest verifies the VCEK chain and the report signature in one call, so a failure here is reported as a
	// signature failure even when the cause is a certificate.
	snpReport := proto.Clone(ev.report).(*spb.Attestation)
	if err := verify.SnpAttestationContext(ctx, snpReport, &verify.Options{DisableCertFetching: true, Now: at}); err != nil {
		return fmt.Errorf("%w: SNP report: %v", ErrSignature, err)
	}

	if len(rootCert) == 0 {
		rootCert = rootPEM
	}
	roots, err := loadRoots(rootCert)
	if err != nil {
		return err
	}
	ak, err := x509.ParseCertificate(ev.vtpm.GetAkCert())
	if err != nil {
		return fmt.Errorf("%w: AK certificate: %v", ErrBadDocument, err)
	}
	inter := x509.NewCertPool()
	for _, der := range ev.vtpm.GetIntermediateCerts() {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return fmt.Errorf("%w: AK intermediate: %v", ErrBadDocument, err)
		}
		inter.AddCert(c)
	}
	// Go's x509 leaves a critical SubjectAltName "unhandled" when it holds none of the name types it parses, which a TPM
	// certificate's does not. go-tpm-tools' VerifyAKCert drops it for the same reason; any other unhandled critical
	// extension still fails the check.
	var unhandled []asn1.ObjectIdentifier
	for _, id := range ak.UnhandledCriticalExtensions {
		if !id.Equal(oidSubjectAltName) {
			unhandled = append(unhandled, id)
		}
	}
	ak.UnhandledCriticalExtensions = unhandled
	if _, err := ak.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, CurrentTime: at, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		return fmt.Errorf("%w: AK certificate: %v", ErrChain, err)
	}

	// The quote is checked against its own extraData here; CheckNonce compares that with the nonce sent.
	extra, err := quoteExtraData(ev)
	if err != nil {
		return err
	}
	if _, err := server.VerifyAttestation(proto.Clone(ev.vtpm).(*attest.Attestation), server.VerifyOpts{
		Nonce:      extra,
		TrustedAKs: []crypto.PublicKey{ak.PublicKey},
		Loader:     server.GRUB,
		HashAlgo:   tpm2.AlgSHA256,
	}); err != nil {
		return fmt.Errorf("%w: vTPM quote: %v", ErrSignature, err)
	}

	if !ev.bound() {
		return fmt.Errorf("%w: REPORT_DATA is not SHA-512 of the vTPM attestation, so the AMD signature does not cover it", ErrSignature)
	}
	return nil
}

// quoteExtraData returns the extraData every quote carries (the nonce the vTPM was given). All quotes must carry the
// same value; a set that disagrees is malformed.
func quoteExtraData(ev evidence) ([]byte, error) {
	var extra []byte
	for i, q := range ev.vtpm.GetQuotes() {
		ad, err := tpm2.DecodeAttestationData(q.GetQuote())
		if err != nil {
			return nil, fmt.Errorf("%w: quote %d: %v", ErrBadDocument, i, err)
		}
		if i > 0 && string(ad.ExtraData) != string(extra) {
			return nil, fmt.Errorf("%w: quotes carry different nonces", ErrBadDocument)
		}
		extra = ad.ExtraData
	}
	return extra, nil
}

func sha512Sum(b []byte) []byte {
	s := sha512.Sum512(b)
	return s[:]
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
