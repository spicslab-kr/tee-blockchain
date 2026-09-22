package aws

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	p "tb/internal/tee/parse/Vendor"
	r "tb/internal/tee/request/Vendor"
)

// Parse — 값이 있는 PCR 을 PCR<n> 이름으로. 전부 0 인 레지스터는 쓰이지 않은 것이라 뺀다.
func (a Parser) Parse(ra r.RADoc) (p.Measurements, error) {
	_, doc, err := decode(ra)
	if err != nil {
		return nil, err
	}
	idx := make([]int, 0, len(doc.PCRs))
	for i := range doc.PCRs {
		idx = append(idx, int(i))
	}
	sort.Ints(idx)
	var out p.Measurements
	for _, i := range idx {
		v := doc.PCRs[uint(i)]
		if bytes.Equal(v, make([]byte, 48)) {
			continue
		}
		out = append(out, p.Measurement{Name: fmt.Sprintf("PCR%d", i), Value: v})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: every PCR is zero", p.ErrBadDocument)
	}
	return out, nil
}

// Info — 시각 · 난수 · 디버그 여부. 디버그 모드 엔클레이브는 PCR0 이 전부 0 이다.
func (a Parser) Info(ra r.RADoc) (p.Info, error) {
	_, doc, err := decode(ra)
	if err != nil {
		return p.Info{}, err
	}
	return p.Info{
		Format:    Format,
		Timestamp: time.UnixMilli(int64(doc.Timestamp)).UTC(),
		Nonce:     doc.Nonce,
		UserData:  doc.UserData,
		Debug:     bytes.Equal(doc.PCRs[0], make([]byte, 48)),
		Module:    doc.ModuleID,
	}, nil
}
