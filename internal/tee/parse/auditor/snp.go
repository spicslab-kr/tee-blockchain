// AMD SEV-SNP parser for the amd-sev-snp-initrd and amd-sev-snp-verity formats. It wraps attestamp/internal/tee/parse/snp
// (the role-independent logic: the evidence container, AMD and vTPM signature checks, the binding, boot-value extraction)
// with auditor's RADoc and Measurements types. The logic is identical for all three roles and lives in one place; this
// adapter converts the types and errors.
package auditor

import (
	"context"
	"errors"
	"fmt"
	"time"

	shared "attestamp/internal/tee/parse/snp"
	r "attestamp/internal/tee/request/auditor"
)

// SNPInitrdFormat and SNPVerityFormat are the document formats the AMD SEV-SNP parser reads. The evidence is the same;
// the measurements registered differ (see internal/tee/parse/snp/measure.go).
const (
	SNPInitrdFormat = shared.FormatInitrd
	SNPVerityFormat = shared.FormatVerity
)

// SNPRootPEM is the built-in root for the cloud vTPM's attestation key. It is used when SNPParser.VerifySig is given an empty certificate.
func SNPRootPEM() []byte { return shared.RootPEM() }

// SNPParser parses amd-sev-snp-* evidence. If Now is zero, the certificate chains are judged at the current time: neither document has a time of its own.
type SNPParser struct {
	Now time.Time
}

var _ Parser = (*SNPParser)(nil)

func (a SNPParser) VerifySig(ctx context.Context, ra r.RADoc, rootCert []byte) error {
	return snpTranslate(shared.Parser{Now: a.Now}.VerifySig(ctx, ra.Format, ra.Raw, rootCert))
}

func (a SNPParser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return snpTranslate(shared.Parser{}.CheckNonce(ra.Format, ra.Raw, nonce))
}

func (a SNPParser) Parse(ra r.RADoc) (Measurements, error) {
	ms, err := (shared.Parser{}).Parse(ra.Format, ra.Raw)
	if err != nil {
		return nil, snpTranslate(err)
	}
	out := make(Measurements, len(ms))
	for i, m := range ms {
		out[i] = Measurement{Name: m.Name, Value: m.Value}
	}
	return out, nil
}

func (a SNPParser) Info(ra r.RADoc) (Info, error) {
	info, err := (shared.Parser{Now: a.Now}).Info(ra.Format, ra.Raw)
	if err != nil {
		return Info{}, snpTranslate(err)
	}
	return Info{
		Format:    info.Format,
		Timestamp: info.Timestamp,
		Nonce:     info.Nonce,
		UserData:  info.UserData,
		Debug:     info.Debug,
		Module:    info.Module,
	}, nil
}

// snpTranslate converts the shared package's errors into this role's error values, so errors.Is(err, ErrXxx) keeps working.
func snpTranslate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, shared.ErrWrongFormat):
		return fmt.Errorf("%w: %v", ErrWrongFormat, err)
	case errors.Is(err, shared.ErrSignature):
		return fmt.Errorf("%w: %v", ErrSignature, err)
	case errors.Is(err, shared.ErrChain):
		return fmt.Errorf("%w: %v", ErrChain, err)
	case errors.Is(err, shared.ErrNonce):
		return fmt.Errorf("%w: %v", ErrNonce, err)
	case errors.Is(err, shared.ErrBadDocument):
		return fmt.Errorf("%w: %v", ErrBadDocument, err)
	default:
		return err
	}
}
