// Package user 는 RA 요청 — 난수를 보내고 RA 문서를 받는다.
package user

import (
	"context"
	"errors"
	"fmt"
)

// RADoc 은 TEE 가 돌려준 RA 문서 원본. 여기서는 열어 보지 않는다.
// Format 은 문서 형식 이름 — 어느 파서로 열어야 하는지. 예: aws-nitro, gcp-cs-jwt, azure-maa-jwt
type RADoc struct {
	Format string
	Raw    []byte
}

type Requester interface {
	Request(ctx context.Context, nonce Nonce, teeAddr string) (RADoc, error)
}

// Extractor 는 TEE 안에서 돈다 — 난수를 넣어 그 TEE 의 RA 문서를 그대로 꺼내 온다.
type Extractor interface {
	Extract(ctx context.Context, nonce Nonce) (RADoc, error)
}

var (
	ErrBadRequest    = errors.New("malformed request")
	ErrBadResponse   = errors.New("malformed response")
	ErrTooLarge      = errors.New("message too large")
	ErrExtractFailed = errors.New("TEE could not produce an attestation document")
	ErrWrongFormat   = errors.New("attestation document has a different format than expected")
)

// ExpectFormat 은 문서 형식이 want 인지 확인한다. want 가 비어 있으면 어떤 형식이든 받는다.
func (d RADoc) ExpectFormat(want string) error {
	if want != "" && d.Format != want {
		return fmt.Errorf("%w: got %q, want %q", ErrWrongFormat, d.Format, want)
	}
	return nil
}
