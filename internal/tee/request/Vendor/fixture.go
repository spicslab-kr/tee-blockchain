package vendor

import (
	"context"
	"os"
)

// FixtureExtractor 는 저장된 문서를 그대로 돌려준다. 난수와 묶이지 않으므로 실제 증명이 아니다 — 테스트 · 드라이런 전용.
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
