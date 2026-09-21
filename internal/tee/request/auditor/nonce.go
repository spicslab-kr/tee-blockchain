package auditor

import (
	"context"
	"crypto/rand"
	"fmt"
)

const NonceSize = 32

type Nonce []byte

// NonceSource 는 난수 생성기. 요구에 따라 HSM · KMS · 외부 RNG 로 바꿔 끼운다.
type NonceSource interface {
	Generate(ctx context.Context) (Nonce, error)
}

// CryptoRand 는 기본 생성기 — OS 의 암호학적 난수(crypto/rand).
type CryptoRand struct{}

func (CryptoRand) Generate(ctx context.Context) (Nonce, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := make([]byte, NonceSize)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	return Nonce(b), nil
}

// DefaultNonceSource 는 아무것도 지정하지 않았을 때 쓰는 생성기.
var DefaultNonceSource NonceSource = CryptoRand{}

// NonceGen 은 기본 생성기로 난수 하나를 만든다.
func NonceGen() (Nonce, error) { return DefaultNonceSource.Generate(context.Background()) }

func (n Nonce) Validate() error {
	if len(n) != NonceSize {
		return fmt.Errorf("nonce: want %d bytes, got %d", NonceSize, len(n))
	}
	return nil
}
