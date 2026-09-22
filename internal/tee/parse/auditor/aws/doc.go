// Package aws 는 AWS Nitro 증명 문서를 읽는다. 문서는 COSE_Sign1 (CBOR) 이고, 안에 PCR · 난수 · 인증서 체인이 들어 있다.
// 서명은 ES384 (ECDSA P-384 + SHA-384), 잎 인증서는 AWS Nitro 루트까지 이어져야 한다.
//
//	doc.go     문서 형식 — CBOR 구조체와 디코딩
//	verify.go  서명 · 인증서 체인 확인
//	nonce.go   난수 확인
//	measure.go 측정값 · 부가 정보 추출
package aws

import (
	_ "embed"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	p "tb/internal/tee/parse/auditor"
	r "tb/internal/tee/request/auditor"
)

// Format 은 이 파서가 읽는 문서 형식. tee/request 의 aws 가 붙이는 이름과 같다.
const Format = "aws-nitro"

//go:embed root.pem
var rootPEM []byte

// RootPEM 은 내장된 AWS Nitro Enclaves 루트 인증서. VerifySig 의 key 를 비우면 이것을 쓴다.
func RootPEM() []byte { return rootPEM }

// coseSign1 — [protected, unprotected, payload, signature]
type coseSign1 struct {
	_           struct{} `cbor:",toarray"`
	Protected   []byte
	Unprotected cbor.RawMessage
	Payload     []byte
	Signature   []byte
}

// document — 페이로드. AWS Nitro Enclaves 문서 규격의 필드 이름 그대로.
type document struct {
	ModuleID    string          `cbor:"module_id"`
	Digest      string          `cbor:"digest"`
	Timestamp   uint64          `cbor:"timestamp"`
	PCRs        map[uint][]byte `cbor:"pcrs"`
	Certificate []byte          `cbor:"certificate"`
	CABundle    [][]byte        `cbor:"cabundle"`
	PublicKey   []byte          `cbor:"public_key"`
	UserData    []byte          `cbor:"user_data"`
	Nonce       []byte          `cbor:"nonce"`
}

// decode — CBOR 을 풀고 필수값을 확인한다. 서명은 보지 않는다.
func decode(ra r.RADoc) (coseSign1, document, error) {
	var cose coseSign1
	var doc document
	if ra.Format != "" && ra.Format != Format {
		return cose, doc, fmt.Errorf("%w: %s", p.ErrWrongFormat, ra.Format)
	}
	if err := cbor.Unmarshal(ra.Raw, &cose); err != nil {
		return cose, doc, fmt.Errorf("%w: not COSE_Sign1: %v", p.ErrBadDocument, err)
	}
	if err := cbor.Unmarshal(cose.Payload, &doc); err != nil {
		return cose, doc, fmt.Errorf("%w: payload: %v", p.ErrBadDocument, err)
	}
	switch {
	case doc.ModuleID == "":
		return cose, doc, fmt.Errorf("%w: no module_id", p.ErrBadDocument)
	case doc.Timestamp == 0:
		return cose, doc, fmt.Errorf("%w: no timestamp", p.ErrBadDocument)
	case doc.Digest != "SHA384":
		return cose, doc, fmt.Errorf("%w: digest %q", p.ErrBadDocument, doc.Digest)
	case len(doc.PCRs) == 0:
		return cose, doc, fmt.Errorf("%w: no pcrs", p.ErrBadDocument)
	case len(doc.Certificate) == 0:
		return cose, doc, fmt.Errorf("%w: no certificate", p.ErrBadDocument)
	}
	for i, v := range doc.PCRs {
		if len(v) != 48 {
			return cose, doc, fmt.Errorf("%w: PCR%d has %d bytes, want 48", p.ErrBadDocument, i, len(v))
		}
	}
	return cose, doc, nil
}
