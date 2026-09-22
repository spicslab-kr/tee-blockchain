// AWS Nitro 파서. user 의 RADoc · Measurements 타입으로 tb/internal/tee/parse/aws (역할과 무관한 실제
// 로직 — CBOR 디코딩, 인증서 체인 · 서명 확인, PCR 추출) 를 감싼다. 로직 자체는 세 역할이 완전히 같아서
// 한 곳에만 있고, 여기는 타입을 맞바꾸는 얇은 어댑터다.
package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	shared "tb/internal/tee/parse/aws"
	r "tb/internal/tee/request/user"
)

// AWSFormat 은 AWS Nitro 파서가 읽는 문서 형식.
const AWSFormat = shared.Format

// AWSRootPEM 은 내장된 AWS Nitro Enclaves 루트 인증서. AWSParser.VerifySig 의 key 를 비우면 이것을 쓴다.
func AWSRootPEM() []byte { return shared.RootPEM() }

// AWSParser — Now 가 0 이면 인증서 체인을 문서의 시각에서 판단한다.
type AWSParser struct {
	Now time.Time
}

var _ Parser = (*AWSParser)(nil)

func (a AWSParser) VerifySig(ctx context.Context, ra r.RADoc, key []byte) error {
	return awsTranslate(shared.Parser{Now: a.Now}.VerifySig(ctx, ra.Format, ra.Raw, key))
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

// awsTranslate — 공통 패키지의 에러를 이 역할의 에러 값으로 바꿔, errors.Is(err, ErrXxx) 가 그대로 통하게 한다.
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
