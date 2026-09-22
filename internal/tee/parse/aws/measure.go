package aws

import (
	"bytes"
	"fmt"
	"sort"
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

// Parse — 값이 있는 PCR 을 PCR<n> 이름으로. 전부 0 인 레지스터는 쓰이지 않은 것이라 뺀다.
func (a Parser) Parse(format string, raw []byte) (Measurements, error) {
	_, doc, err := decode(format, raw)
	if err != nil {
		return nil, err
	}
	idx := make([]int, 0, len(doc.PCRs))
	for i := range doc.PCRs {
		idx = append(idx, int(i))
	}
	sort.Ints(idx)
	var out Measurements
	for _, i := range idx {
		v := doc.PCRs[uint(i)]
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
