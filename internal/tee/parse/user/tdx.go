// Intel TDX parser for the intel-tdx format. It wraps attestamp/internal/tee/parse/tdx (the role-independent logic: quote
// decoding, PCK chain and signature verification, MRTD and RTMR extraction) with user's RADoc and Measurements types.
// The logic is identical for all three roles and lives in one place; this adapter converts the types and errors.

package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	shared "attestamp/internal/tee/parse/tdx"
	r "attestamp/internal/tee/request/user"
)

// TDXFormat is the document format the Intel TDX parser reads.
const TDXFormat = shared.Format

// TDXRootPEM is the built-in Intel SGX Root CA certificate. It is used when TDXParser.VerifySig is given an empty certificate.
func TDXRootPEM() []byte { return shared.RootPEM() }

// TDXParser parses intel-tdx quotes. If Now is zero, the certificate chain is judged at the current time: a quote has no time of its own.
type TDXParser struct {
	Now time.Time
}

var _ Parser = (*TDXParser)(nil)

func (a TDXParser) VerifySig(ctx context.Context, ra r.RADoc, rootCert []byte) error {
	return tdxTranslate(shared.Parser{Now: a.Now}.VerifySig(ctx, ra.Format, ra.Raw, rootCert))
}

func (a TDXParser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return tdxTranslate(shared.Parser{}.CheckNonce(ra.Format, ra.Raw, nonce))
}

func (a TDXParser) Parse(ra r.RADoc) (Measurements, error) {
	ms, err := (shared.Parser{}).Parse(ra.Format, ra.Raw)
	if err != nil {
		return nil, tdxTranslate(err)
	}
	out := make(Measurements, len(ms))
	for i, m := range ms {
		out[i] = Measurement{Name: m.Name, Value: m.Value}
	}
	return out, nil
}

func (a TDXParser) Info(ra r.RADoc) (Info, error) {
	info, err := (shared.Parser{Now: a.Now}).Info(ra.Format, ra.Raw)
	if err != nil {
		return Info{}, tdxTranslate(err)
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

// tdxTranslate converts the shared package's errors into this role's error values, so errors.Is(err, ErrXxx) keeps working.
func tdxTranslate(err error) error {
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
