// Package user 는 C2 체인 조회 — 읽기만 한다.
//
// auditor · Vendor · user 세 역할에 이 파일이 똑같이 하나씩 있다. Registration.Values 와
// Reader.FindAddress 가 이 역할의 tee/parse/user.Measurements 를 쓰는데, auditor·Vendor 의
// Measurements 는 필드가 똑같아도(Name, Value) 패키지가 달라서 Go 에서는 다른 타입이다 — 그래서
// 인터페이스 하나를 세 역할이 같이 쓸 수가 없다. Meta 자체는 역할 타입이 없어 합칠 수도 있었지만,
// Registration·Reader 와 한 파일에 있는 게 자연스러워 그냥 같이 뒀다.
//
// 실제 구현(ether/reader.go)도 마찬가지 이유다. user 는 등록을 안 해서 chain/register/user 가 없다 —
// ether/reader.go 가 auditor 의 컨트랙트 바인딩을 빌려 쓰는 이유는 그 파일 자체의 주석에 있다.
package user

import (
	"context"

	"tb/internal/tee/parse/user"
)

type Meta struct {
	Service  string
	Version  string
	Platform string
	Arch     string // 이미지를 빌드한 CPU 아키텍처 (amd64 | arm64). 표시용 — 키에는 안 들어간다
}

type Registration struct {
	Address   string
	Values    user.Measurements
	Meta      Meta
	Expires   int64 // 체인 시계 기준
	Signer    string
	Signature []byte
}

type Reader interface {
	FindAddress(ctx context.Context, values user.Measurements) ([]string, error)
	Lookup(ctx context.Context, address string) (Registration, error)
}
