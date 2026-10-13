package auditor

import (
	"context"
	"os"
)

// FixtureExtractor returns a stored document as is. It is not bound to the nonce, so it is not a real attestation; for tests and dry runs only.
type FixtureExtractor struct {
	Format string
	Doc    []byte
}

var _ Extractor = (*FixtureExtractor)(nil)

func (f FixtureExtractor) Extract(ctx context.Context, _ Nonce) (RADoc, error) {
	if err := ctx.Err(); err != nil {
		return RADoc{}, err
	}
	return RADoc{Format: f.Format, Raw: f.Doc}, nil
}

func LoadFixture(platform, path string) (FixtureExtractor, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FixtureExtractor{}, err
	}
	return FixtureExtractor{Format: platform, Doc: b}, nil
}
