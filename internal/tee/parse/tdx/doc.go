// Package tdx reads Intel TDX quotes (format intel-tdx). A quote is the binary structure from Intel's TDX DCAP
// quote format: a header, the TD quote body (MRTD, RTMR0-3, REPORTDATA and the rest of the TD report) and an
// ECDSA P-256 signature by an attestation key. The Quoting Enclave vouches for that key with a report signed by the
// PCK certificate, and the PCK certificate must chain up to the Intel SGX Root CA.
//
// The logic is identical for every role (auditor, Vendor, user), so it lives here once. Each role's
// internal/tee/parse/<role>/tdx.go is only a thin adapter that wraps this package with that role's RADoc, Nonce and
// Measurements types.
//
// Decoding and the quote signature checks come from github.com/google/go-tdx-guest. The certificate chain is checked
// here as well, before the library runs, so a wrong root is reported as ErrChain and not mixed up with a bad signature.
//
//	doc.go     quote format: decoding and the required values
//	verify.go  PCK certificate chain and signature verification
//	nonce.go   nonce check against REPORTDATA
//	measure.go measurement and extra-info extraction
package tdx

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/google/go-tdx-guest/abi"
	pb "github.com/google/go-tdx-guest/proto/tdx"
)

// Format is the document format this parser reads; it is the same name that tdx in tee/request attaches.
const Format = "intel-tdx"

// root.pem is the Intel SGX Root CA. The same certificate is embedded in go-tdx-guest (verify/trusted_root.pem) and
// is the third certificate of the PCK chain inside a real GCP quote (sha256 fingerprint 44:A0:19:6B:...:74:D3).
//
//go:embed root.pem
var rootPEM []byte

// RootPEM is the built-in Intel SGX Root CA certificate. It is used when VerifySig is given an empty certificate.
func RootPEM() []byte { return rootPEM }

var (
	ErrWrongFormat = errors.New("document format does not match this parser")
	ErrBadDocument = errors.New("document is malformed")
	ErrSignature   = errors.New("document signature does not verify")
	ErrChain       = errors.New("document certificate does not chain to the platform root")
	ErrNonce       = errors.New("document nonce is not the one sent")
)

// measurementSize is the size of MRTD and of each RTMR: SHA-384.
const measurementSize = 48

// decode unpacks the quote and checks the required values. It does not check the signature.
//
// Only quote version 4 is accepted. It is what GCP TDX guests return through ConfigFS TSM today, and the only
// version go-tdx-guest v0.3.1 decodes; version 5 needs a newer library and its own fixture.
func decode(format string, raw []byte) (*pb.QuoteV4, error) {
	if format != "" && format != Format {
		return nil, fmt.Errorf("%w: %s", ErrWrongFormat, format)
	}
	q, err := abi.QuoteToProto(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadDocument, err)
	}
	v4, ok := q.(*pb.QuoteV4)
	if !ok {
		return nil, fmt.Errorf("%w: quote type %T, want version 4", ErrBadDocument, q)
	}
	if err := abi.CheckQuoteV4(v4); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadDocument, err)
	}
	if t := v4.GetHeader().GetTeeType(); t != abi.TeeTDX {
		return nil, fmt.Errorf("%w: TEE type %#x, want %#x (TDX)", ErrBadDocument, t, abi.TeeTDX)
	}
	body := v4.GetTdQuoteBody()
	if len(body.GetMrTd()) != measurementSize || len(body.GetRtmrs()) != 4 {
		return nil, fmt.Errorf("%w: MRTD or RTMRs missing", ErrBadDocument)
	}
	return v4, nil
}
