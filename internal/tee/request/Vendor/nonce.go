package vendor

import (
	"context"
	"crypto/rand"
	"fmt"
)

const NonceSize = 32

type Nonce []byte

// NonceSource is a nonce generator. It can be swapped for an HSM, a KMS or an external RNG as required.
type NonceSource interface {
	Generate(ctx context.Context) (Nonce, error)
}

// CryptoRand is the default generator: the OS cryptographic random source (crypto/rand).
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

// DefaultNonceSource is the generator used when none is specified.
var DefaultNonceSource NonceSource = CryptoRand{}

// NonceGen makes one nonce with the default generator.
func NonceGen() (Nonce, error) { return DefaultNonceSource.Generate(context.Background()) }

func (n Nonce) Validate() error {
	if len(n) != NonceSize {
		return fmt.Errorf("nonce: want %d bytes, got %d", NonceSize, len(n))
	}
	return nil
}
