package aws

import (
	"bytes"
	"fmt"
	"time"
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

// Info 는 측정값 말고 문서가 말하는 것들. 대조 · 판정에서 만료 · 신선도 · 디버그 여부를 볼 때 쓴다.
type Info struct {
	Format    string
	Timestamp time.Time // 문서가 만들어진 시각. 이 시각에 인증서가 유효했는지로 판단한다
	Nonce     []byte
	UserData  []byte
	Debug     bool // 디버그 모드 TEE — 측정값이 의미가 없다
	Module    string
}

// imagePCRs — 이미지(코드)를 말하는 레지스터. nitro-cli build-enclave 가 내는 것과 같은 셋이라, 여기서 나온
// 값이 빌드값 · 측정값 기록 · 체인 등록값과 그대로 맞대진다. PCR3(IAM 역할) · PCR4(부모 인스턴스 ID) ·
// PCR8(서명 인증서)은 어느 기계에서 돌았나이지 무엇이 돌았나가 아니라서 뺀다 — 인스턴스는 Info.Module 로 본다.
var imagePCRs = []uint{0, 1, 2}

// Parse — PCR0 · 1 · 2 를 PCR<n> 이름으로. 전부 0 인 레지스터(디버그 모드)는 뺀다.
func (a Parser) Parse(format string, raw []byte) (Measurements, error) {
	_, doc, err := decode(format, raw)
	if err != nil {
		return nil, err
	}
	var out Measurements
	for _, i := range imagePCRs {
		v := doc.PCRs[i]
		if bytes.Equal(v, make([]byte, 48)) {
			continue
		}
		out = append(out, Measurement{Name: fmt.Sprintf("PCR%d", i), Value: v})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: every PCR is zero", ErrBadDocument)
	}
	return out, nil
}

// Info — 시각 · 난수 · 디버그 여부. 디버그 모드 엔클레이브는 PCR0 이 전부 0 이다.
func (a Parser) Info(format string, raw []byte) (Info, error) {
	_, doc, err := decode(format, raw)
	if err != nil {
		return Info{}, err
	}
	return Info{
		Format:    Format,
		Timestamp: time.UnixMilli(int64(doc.Timestamp)).UTC(),
		Nonce:     doc.Nonce,
		UserData:  doc.UserData,
		Debug:     bytes.Equal(doc.PCRs[0], make([]byte, 48)),
		Module:    doc.ModuleID,
	}, nil
}
