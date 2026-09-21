// Package auditor 는 체인 등록 — 측정값을 서명해 컨트랙트에 기록한다.
package auditor

import (
	"context"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	l "tb/internal/chain/lookup/auditor"
	p "tb/internal/tee/parse/auditor"

	"golang.org/x/crypto/sha3"
)

// Meta 는 측정값에 붙는 등록 정보. Expires 가 지나면 등록은 효력을 잃는다. 취소는 없다.
type Meta struct {
	l.Meta
	Expires time.Time
}

// Signed 는 서명이 끝나 체인에 보낼 수 있는 값.
type Signed struct {
	Values    p.Measurements
	Meta      Meta
	Signer    string // 서명 키의 주소. 컨트랙트가 서명에서 복원하는 값과 같다
	Digest    []byte // 서명한 해시. 컨트랙트의 digest() 와 같아야 한다
	Signature []byte // 65 바이트 r‖s‖v
}

// Receipt 는 등록 완료. Key 가 조회 좌표다.
type Receipt struct {
	Key    string // 0x… 32 바이트
	TxHash string
	Block  uint64
}

type Registrar interface {
	// Sign 은 측정값과 등록 정보를 서명 키로 서명한다. 누가 등록했는지는 이 서명으로 갈린다.
	Sign(values p.Measurements, meta Meta, key []byte) (Signed, error)
	// Register 는 서명된 값을 컨트랙트에 보낸다. 컨트랙트가 서명자를 확인한 뒤 저장한다.
	Register(ctx context.Context, s Signed) (Receipt, error)
}

var (
	ErrNotAnApprover     = errors.New("signer is not one of the approvers fixed in the contract")
	ErrAlreadyRegistered = errors.New("this signer already registered this measurement")
	ErrExpiryPassed      = errors.New("expiry is not in the future by the chain's clock")
)

// MeasurementHash 는 측정값 묶음을 32 바이트 하나로 줄인다. 컨트랙트에는 이 값이 들어간다.
// 이름순으로 "이름=값(hex)\n" 을 이어 붙여 keccak256. 같은 측정값이면 누가 계산해도 같다.
func MeasurementHash(values p.Measurements) ([32]byte, error) {
	var out [32]byte
	if len(values) == 0 {
		return out, errors.New("no measurements")
	}
	sorted := make(p.Measurements, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha3.NewLegacyKeccak256()
	for i, m := range sorted {
		if m.Name == "" || len(m.Value) == 0 {
			return out, errors.New("measurement with empty name or value")
		}
		if i > 0 && sorted[i-1].Name == m.Name {
			return out, errors.New("duplicate measurement name: " + m.Name)
		}
		h.Write([]byte(m.Name))
		h.Write([]byte("="))
		h.Write([]byte(hex.EncodeToString(m.Value)))
		h.Write([]byte("\n"))
	}
	copy(out[:], h.Sum(nil))
	return out, nil
}

func (m Meta) Validate() error {
	switch {
	case m.Platform == "":
		return errors.New("meta: platform is empty")
	case m.Service == "":
		return errors.New("meta: service is empty")
	case m.Version == "":
		return errors.New("meta: version is empty")
	case m.Expires.IsZero():
		return errors.New("meta: expires is empty")
	}
	return nil
}
