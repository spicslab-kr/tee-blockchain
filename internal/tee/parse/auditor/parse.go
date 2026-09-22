// Package auditor 는 RA 파싱 — 서명과 난수를 확인하고 측정값만 꺼낸다.
package auditor

import (
	"context"
	"errors"
	"time"

	r "tb/internal/tee/request/auditor"
)

type Measurement struct {
	Name  string
	Value []byte
}

type Measurements []Measurement

// Get 은 이름으로 값을 찾는다. 없으면 nil.
func (ms Measurements) Get(name string) []byte {
	for _, m := range ms {
		if m.Name == name {
			return m.Value
		}
	}
	return nil
}

// Parser 는 플랫폼마다 하나. key 는 그 플랫폼의 검증 키(예: Nitro 루트 인증서 PEM). 비우면 내장된 것.
type Parser interface {
	// VerifySig 는 문서가 그 플랫폼이 서명한 것인지 본다. 서명과 인증서 체인.
	VerifySig(ctx context.Context, ra r.RADoc, key []byte) error
	// CheckNonce 는 문서 안의 난수가 내가 보낸 것인지 본다.
	CheckNonce(ra r.RADoc, nonce r.Nonce) error
	// Parse 는 측정값을 꺼낸다. 서명은 보지 않는다 — VerifySig 를 먼저 통과시켜야 한다.
	Parse(ra r.RADoc) (Measurements, error)
}

// Info 는 측정값 말고 문서가 말하는 것들. 대조 · 판정에서 만료 · 신선도 · 디버그 여부를 볼 때 쓴다.
type Info struct {
	Format    string
	Timestamp time.Time // 문서가 만들어진 시각. 이 시각에 인증서가 유효했는지로 판단한다
	Nonce     []byte
	UserData  []byte
	Debug     bool // 디버그 모드 TEE — 측정값이 의미가 없다
	Module    string
}

var (
	ErrWrongFormat = errors.New("document format does not match this parser")
	ErrBadDocument = errors.New("document is malformed")
	ErrSignature   = errors.New("document signature does not verify")
	ErrChain       = errors.New("document certificate does not chain to the platform root")
	ErrNonce       = errors.New("document nonce is not the one sent")
)
