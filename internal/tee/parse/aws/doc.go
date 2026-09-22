// Package aws 는 AWS Nitro 증명 문서를 읽는다. 문서는 COSE_Sign1 (CBOR) 이고, 안에 PCR · 난수 · 인증서 체인이 들어 있다.
// 서명은 ES384 (ECDSA P-384 + SHA-384), 잎 인증서는 AWS Nitro 루트까지 이어져야 한다.
//
// 역할(auditor · Vendor · user)에 상관없이 로직이 완전히 같아 여기 하나로 둔다. 각 역할의
// internal/tee/parse/<역할>/aws 는 자기 역할의 RADoc · Nonce · Measurements 타입으로 이 패키지를 감싸는
// 얇은 어댑터일 뿐이다.
//
//	doc.go     문서 형식 — CBOR 구조체와 디코딩
//	verify.go  서명 · 인증서 체인 확인
//	nonce.go   난수 확인
//	measure.go 측정값 · 부가 정보 추출
package aws

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// Format 은 이 파서가 읽는 문서 형식. tee/request 의 aws 가 붙이는 이름과 같다.
const Format = "aws-nitro"

//go:embed root.pem
var rootPEM []byte

// RootPEM 은 내장된 AWS Nitro Enclaves 루트 인증서. VerifySig 의 key 를 비우면 이것을 쓴다.
func RootPEM() []byte { return rootPEM }

var (
	ErrWrongFormat = errors.New("document format does not match this parser")
	ErrBadDocument = errors.New("document is malformed")
	ErrSignature   = errors.New("document signature does not verify")
	ErrChain       = errors.New("document certificate does not chain to the platform root")
	ErrNonce       = errors.New("document nonce is not the one sent")
)

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
func decode(format string, raw []byte) (coseSign1, document, error) {
	var cose coseSign1
	var doc document
	if format != "" && format != Format {
		return cose, doc, fmt.Errorf("%w: %s", ErrWrongFormat, format)
	}
	if err := cbor.Unmarshal(raw, &cose); err != nil {
		return cose, doc, fmt.Errorf("%w: not COSE_Sign1: %v", ErrBadDocument, err)
	}
	if err := cbor.Unmarshal(cose.Payload, &doc); err != nil {
		return cose, doc, fmt.Errorf("%w: payload: %v", ErrBadDocument, err)
	}
	switch {
	case doc.ModuleID == "":
		return cose, doc, fmt.Errorf("%w: no module_id", ErrBadDocument)
	case doc.Timestamp == 0:
		return cose, doc, fmt.Errorf("%w: no timestamp", ErrBadDocument)
	case doc.Digest != "SHA384":
		return cose, doc, fmt.Errorf("%w: digest %q", ErrBadDocument, doc.Digest)
	case len(doc.PCRs) == 0:
		return cose, doc, fmt.Errorf("%w: no pcrs", ErrBadDocument)
	case len(doc.Certificate) == 0:
		return cose, doc, fmt.Errorf("%w: no certificate", ErrBadDocument)
	}
	for i, v := range doc.PCRs {
		if len(v) != 48 {
			return cose, doc, fmt.Errorf("%w: PCR%d has %d bytes, want 48", ErrBadDocument, i, len(v))
		}
	}
	return cose, doc, nil
}
