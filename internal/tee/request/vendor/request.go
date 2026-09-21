// Package vendor 는 T1 RA 요청 — 난수를 보내고 RA 문서를 받는다.
package vendor

import (
	"context"
	"errors"
)

type Nonce []byte

type RADoc struct {
	Platform string
	Raw      []byte
}

type Requester interface {
	Request(ctx context.Context, nonce Nonce, teeAddr string) (RADoc, error)
}

// Extractor 는 TEE 안에 들어가는 추출기 — 그 TEE 의 RA 문서를 그대로 꺼내 온다.
type Extractor interface {
	Extract(ctx context.Context, nonce Nonce) (RADoc, error)
}

const NonceSize = 32

// NonceGen 은 난수 생성 래퍼 하나 — 생성기를 바꿀 때 여기만 고친다.
func NonceGen() (Nonce, error) {
	return nil, errors.New("tee/request/vendor: NonceGen not implemented")
}
