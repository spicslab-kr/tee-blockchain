package aws

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"tb/internal/tee/request/auditor"
)

const Format = "aws-nitro"

// NSM 요청 — {"Attestation": {"user_data": .., "nonce": .., "public_key": ..}}
type nsmRequest struct {
	Attestation nsmAttestationReq `cbor:"Attestation"`
}

type nsmAttestationReq struct {
	UserData  []byte `cbor:"user_data"`
	Nonce     []byte `cbor:"nonce"`
	PublicKey []byte `cbor:"public_key"`
}

// NSM 응답 — {"Attestation": {"document": ..}} 또는 {"Error": "..."}
type nsmResponse struct {
	Attestation *struct {
		Document []byte `cbor:"document"`
	} `cbor:"Attestation,omitempty"`
	Error *string `cbor:"Error,omitempty"`
}

func encodeAttestationRequest(nonce auditor.Nonce, userData []byte) ([]byte, error) {
	if err := nonce.Validate(); err != nil {
		return nil, err
	}
	return cbor.Marshal(nsmRequest{Attestation: nsmAttestationReq{UserData: userData, Nonce: nonce}})
}

func decodeAttestationResponse(b []byte) ([]byte, error) {
	var r nsmResponse
	if err := cbor.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("nsm response: %w", err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("nsm: %s", *r.Error)
	}
	if r.Attestation == nil || len(r.Attestation.Document) == 0 {
		return nil, errors.New("nsm response: no document")
	}
	return r.Attestation.Document, nil
}
