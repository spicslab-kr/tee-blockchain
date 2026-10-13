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

// Get finds a value by name, or returns nil.
func (ms Measurements) Get(name string) []byte {
	for _, m := range ms {
		if m.Name == name {
			return m.Value
		}
	}
	return nil
}

// Info is what the document says besides the measurements. Comparison uses it to check expiry, freshness and debug mode.
type Info struct {
	Format    string
	Timestamp time.Time // when the document was created; certificates are judged valid as of this time
	Nonce     []byte
	UserData  []byte
	Debug     bool // debug-mode TEE: the measurements are meaningless
	Module    string
}

// imagePCRs are the registers that describe the image (the code). They are the same three that nitro-cli
// build-enclave outputs, so values from here compare directly against the build values, the measurement record
// and the on-chain registration. PCR3 (IAM role), PCR4 (parent instance ID) and PCR8 (signing certificate) say
// which machine it ran on, not what ran, so they are left out; the instance is available as Info.Module.
var imagePCRs = []uint{0, 1, 2}

// Parse returns PCR0, 1 and 2 under the names PCR<n>. An all-zero register (debug mode) is left out.
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

// Info returns the time, nonce and debug flag. A debug-mode enclave has an all-zero PCR0.
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
