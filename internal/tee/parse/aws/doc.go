// Package aws reads AWS Nitro attestation documents (format aws-nitro). A document is COSE_Sign1 (CBOR) containing the PCRs, a nonce
// and a certificate chain. The signature is ES384 (ECDSA P-384 + SHA-384), and the leaf certificate must chain up to the AWS Nitro root.
//
// The logic is identical for every role (auditor, Vendor, user), so it lives here once. Each role's
// internal/tee/parse/<role>/aws is only a thin adapter that wraps this package with that role's RADoc, Nonce and
// Measurements types.
//
//	doc.go     document format: CBOR structures and decoding
//	verify.go  signature and certificate chain verification
//	nonce.go   nonce check
//	measure.go measurement and extra-info extraction
package aws

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// Format is the document format this parser reads; it is the same name that aws in tee/request attaches.
const Format = "aws-nitro"

//go:embed root.pem
var rootPEM []byte

// RootPEM is the built-in AWS Nitro Enclaves root certificate. It is used when VerifySig is given an empty certificate.
func RootPEM() []byte { return rootPEM }

var (
	ErrWrongFormat = errors.New("document format does not match this parser")
	ErrBadDocument = errors.New("document is malformed")
	ErrSignature   = errors.New("document signature does not verify")
	ErrChain       = errors.New("document certificate does not chain to the platform root")
	ErrNonce       = errors.New("document nonce is not the one sent")
)

// coseSign1 is [protected, unprotected, payload, signature]
type coseSign1 struct {
	_           struct{} `cbor:",toarray"`
	Protected   []byte
	Unprotected cbor.RawMessage
	Payload     []byte
	Signature   []byte
}

// document is the payload, with the field names of the AWS Nitro Enclaves document specification.
type document struct {
	ModuleID    string          `cbor:"module_id"`
	Digest      string          `cbor:"digest"`
	Timestamp   uint64          `cbor:"timestamp"`
	PCRs        map[uint][]byte `cbor:"pcrs"`
	Certificate []byte          `cbor:"certificate"`
	CABundle    [][]byte        `cbor:"cabundle"`
	PublicKey   []byte          `cbor:"public_key"`
	UserData    []byte          `cbor:"user_data"`
	Nonce       []byte          `cbor:"nonce"`
}

// decode unpacks the CBOR and checks the required values. It does not check the signature.
func decode(format string, raw []byte) (coseSign1, document, error) {
	var cose coseSign1
	var doc document
	if format != "" && format != Format {
		return cose, doc, fmt.Errorf("%w: %s", ErrWrongFormat, format)
	}
	if err := cbor.Unmarshal(raw, &cose); err != nil {
		return cose, doc, fmt.Errorf("%w: not COSE_Sign1: %v", ErrBadDocument, err)
	}
	if err := cbor.Unmarshal(cose.Payload, &doc); err != nil {
		return cose, doc, fmt.Errorf("%w: payload: %v", ErrBadDocument, err)
	}
	switch {
	case doc.ModuleID == "":
		return cose, doc, fmt.Errorf("%w: no module_id", ErrBadDocument)
	case doc.Timestamp == 0:
		return cose, doc, fmt.Errorf("%w: no timestamp", ErrBadDocument)
	case doc.Digest != "SHA384":
		return cose, doc, fmt.Errorf("%w: digest %q", ErrBadDocument, doc.Digest)
	case len(doc.PCRs) == 0:
		return cose, doc, fmt.Errorf("%w: no pcrs", ErrBadDocument)
	case len(doc.Certificate) == 0:
		return cose, doc, fmt.Errorf("%w: no certificate", ErrBadDocument)
	}
	for i, v := range doc.PCRs {
		if len(v) != 48 {
			return cose, doc, fmt.Errorf("%w: PCR%d has %d bytes, want 48", ErrBadDocument, i, len(v))
		}
	}
	return cose, doc, nil
}
