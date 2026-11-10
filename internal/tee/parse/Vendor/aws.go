// AWS Nitro parser for the aws-nitro format. It wraps attestamp/internal/tee/parse/aws (the role-independent logic: CBOR decoding, certificate
// chain and signature verification, PCR extraction) with Vendor's RADoc and Measurements types. The logic is identical
// for all three roles and lives in one place; this adapter converts the types and errors.
package vendor

import (
	"context"
	"errors"
	"fmt"
	"time"

	shared "attestamp/internal/tee/parse/aws"
	r "attestamp/internal/tee/request/Vendor"
)

// AWSFormat is the document format the AWS Nitro parser reads.
const AWSFormat = shared.Format

// AWSRootPEM is the built-in AWS Nitro Enclaves root certificate. It is used when AWSParser.VerifySig is given an empty certificate.
func AWSRootPEM() []byte { return shared.RootPEM() }

// AWSParser parses aws-nitro documents. If Now is zero, the certificate chain is judged at the document's own time.
type AWSParser struct {
	Now time.Time
}

var _ Parser = (*AWSParser)(nil)

func (a AWSParser) VerifySig(ctx context.Context, ra r.RADoc, rootCert []byte) error {
	return awsTranslate(shared.Parser{Now: a.Now}.VerifySig(ctx, ra.Format, ra.Raw, rootCert))
}

func (a AWSParser) CheckNonce(ra r.RADoc, nonce r.Nonce) error {
	return awsTranslate(shared.Parser{}.CheckNonce(ra.Format, ra.Raw, nonce))
}

func (a AWSParser) Parse(ra r.RADoc) (Measurements, error) {
	ms, err := (shared.Parser{}).Parse(ra.Format, ra.Raw)
	if err != nil {
		return nil, awsTranslate(err)
	}
	out := make(Measurements, len(ms))
	for i, m := range ms {
		out[i] = Measurement{Name: m.Name, Value: m.Value}
	}
	return out, nil
}

func (a AWSParser) Info(ra r.RADoc) (Info, error) {
	info, err := (shared.Parser{Now: a.Now}).Info(ra.Format, ra.Raw)
	if err != nil {
		return Info{}, awsTranslate(err)
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

// awsTranslate converts the shared package's errors into this role's error values, so errors.Is(err, ErrXxx) keeps working.
func awsTranslate(err error) error {
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
