// Package snp reads AMD SEV-SNP evidence from a cloud Confidential VM (formats amd-sev-snp-initrd and
// amd-sev-snp-verity). On GCP the SNP launch measurement covers only the virtual firmware; the boot files that carry
// the application are measured by the cloud's vTPM. So one piece of evidence is two documents from two signers:
//
//	report  the SNP attestation report, signed by the chip's VCEK (AMD), REPORT_DATA = SHA3-512(vtpm)
//	certs   the certificate table the host returned with the report (VCEK, ASK, ARK), possibly empty
//	vtpm    a vTPM attestation: go-tpm-tools attest.Attestation (binary protobuf), quotes over the PCRs signed by the
//	        cloud's attestation key, the AK certificate and the TCG event log
//
// They travel together as one CBOR map in RADoc.Raw (see Encode). The binding is the SHA3-512: the AMD signature covers
// REPORT_DATA, so it covers the exact vTPM bytes carried next to it. (SHA-512 until 2026-11-11; see bindingSum.)
//
// What the binding does not prove: that both documents come from the same VM. A vTPM attestation from one VM and an
// SNP report from another VM, asked for with the first one's hash, verify together. That was reproduced on GCP and
// AWS (docs/plan/test-matrix.md, T10) and is the reason SEV-SNP is weaker here than Nitro or TDX.
//
// The logic is identical for every role, so it lives here once. Each role's internal/tee/parse/<role>/snp.go is only
// a thin adapter.
//
//	doc.go     evidence format: the CBOR container and decoding
//	verify.go  AMD chain and report signature, cloud AK chain and quote signature, the binding
//	nonce.go   nonce check against the vTPM quotes
//	measure.go measurement and extra-info extraction
package snp

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	sabi "github.com/google/go-sev-guest/abi"
	spb "github.com/google/go-sev-guest/proto/sevsnp"
	attest "github.com/google/go-tpm-tools/proto/attest"
	"google.golang.org/protobuf/proto"
)

// The two formats differ only in which measurements are registered (see measure.go). They are two names rather than
// one because the on-chain measurementHash covers the sorted name/value list, so one name with a varying number of
// values would give hashes that cannot be compared.
const (
	// FormatInitrd: the application is a cpio archive appended to the base initrd (Option 1).
	FormatInitrd = "amd-sev-snp-initrd"
	// FormatVerity: the application is a squashfs image on disk, checked at boot by dm-verity against a root hash on
	// the kernel command line, with the result extended into PCR12 (Option 2).
	FormatVerity = "amd-sev-snp-verity"
)

// root.pem is Google's "EK/AK CA Root" (Google Cloud), the root the GCE vTPM attestation key certificate chains to. It
// is go-tpm-tools' server/ca-certs/gcp_ek_ak_ca_root.crt (sha256 fingerprint 59:47:59:59:...:BE:B2, valid 2022-07-08
// to 2122-07-08). The AMD roots (ARK and ASK per product) are the ones go-sev-guest embeds.
//
//go:embed root.pem
var rootPEM []byte

// RootPEM is the built-in root for the vTPM attestation key. It is used when VerifySig is given an empty certificate.
func RootPEM() []byte { return rootPEM }

var (
	ErrWrongFormat = errors.New("document format does not match this parser")
	ErrBadDocument = errors.New("document is malformed")
	ErrSignature   = errors.New("document signature does not verify")
	ErrChain       = errors.New("document certificate does not chain to the platform root")
	ErrNonce       = errors.New("document nonce is not the one sent")
)

// container is the CBOR map carried in RADoc.Raw. Text keys keep it readable with any CBOR tool; unknown keys are
// refused when decoding, so a field added later cannot be silently ignored by an old verifier.
type container struct {
	Report []byte `cbor:"report"`
	Certs  []byte `cbor:"certs"`
	VTPM   []byte `cbor:"vtpm"`
}

var (
	encMode, _ = cbor.CoreDetEncOptions().EncMode()
	decMode, _ = cbor.DecOptions{ExtraReturnErrors: cbor.ExtraDecErrorUnknownField, DupMapKey: cbor.DupMapKeyEnforcedAPF}.DecMode()
)

// Encode packs one evidence pair into the bytes carried in RADoc.Raw. The encoding is CBOR's core deterministic one, so
// the same three inputs always give the same bytes.
func Encode(report, certs, vtpm []byte) ([]byte, error) {
	return encMode.Marshal(container{Report: report, Certs: certs, VTPM: vtpm})
}

// evidence is a decoded container.
type evidence struct {
	report  *spb.Attestation // report and certificate chain, as go-sev-guest reads them
	vtpm    *attest.Attestation
	vtpmRaw []byte
}

// decode unpacks the container and both documents and checks the required values. It does not check any signature.
func decode(format string, raw []byte) (evidence, error) {
	var ev evidence
	if format != "" && format != FormatInitrd && format != FormatVerity {
		return ev, fmt.Errorf("%w: %s", ErrWrongFormat, format)
	}
	var c container
	if err := decMode.Unmarshal(raw, &c); err != nil {
		return ev, fmt.Errorf("%w: not the SEV-SNP evidence container: %v", ErrBadDocument, err)
	}
	if len(c.Report) != sabi.ReportSize {
		return ev, fmt.Errorf("%w: report is %d bytes, want %d", ErrBadDocument, len(c.Report), sabi.ReportSize)
	}
	if len(c.VTPM) == 0 {
		return ev, fmt.Errorf("%w: no vTPM attestation", ErrBadDocument)
	}
	a, err := sabi.ReportCertsToProto(append(append([]byte(nil), c.Report...), c.Certs...))
	if err != nil {
		return ev, fmt.Errorf("%w: report: %v", ErrBadDocument, err)
	}
	v := &attest.Attestation{}
	if err := proto.Unmarshal(c.VTPM, v); err != nil {
		return ev, fmt.Errorf("%w: vTPM attestation: %v", ErrBadDocument, err)
	}
	if len(v.GetAkCert()) == 0 || len(v.GetQuotes()) == 0 {
		return ev, fmt.Errorf("%w: vTPM attestation has no AK certificate or no quote", ErrBadDocument)
	}
	return evidence{report: a, vtpm: v, vtpmRaw: c.VTPM}, nil
}

// reportData is REPORT_DATA from the report, 64 bytes.
func (ev evidence) reportData() []byte { return ev.report.GetReport().GetReportData() }

// bound reports whether REPORT_DATA is SHA3-512 of the vTPM attestation bytes.
func (ev evidence) bound() bool {
	return bytes.Equal(ev.reportData(), bindingSum(ev.vtpmRaw))
}
