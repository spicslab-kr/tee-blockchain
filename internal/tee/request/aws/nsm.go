// Package aws gets an AWS Nitro attestation document from inside the enclave (/dev/nsm), and carries it between the
// parent instance and the enclave (vsock, the TCP-to-vsock proxy, the enclave's loopback). It does not depend on the
// role: each role's internal/tee/request/<role>/aws checks the nonce and turns the document into that role's RADoc, as
// internal/tee/request/<role>/tdx does over internal/tee/request/tdx.
package aws

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// Format is the document format name of Nitro attestation documents.
const Format = "aws-nitro"

// NSM request: {"Attestation": {"user_data": .., "nonce": .., "public_key": ..}}
type nsmRequest struct {
	Attestation nsmAttestationReq `cbor:"Attestation"`
}

type nsmAttestationReq struct {
	UserData  []byte `cbor:"user_data"`
	Nonce     []byte `cbor:"nonce"`
	PublicKey []byte `cbor:"public_key"`
}

// NSM response: {"Attestation": {"document": ..}} or {"Error": "..."}
type nsmResponse struct {
	Attestation *struct {
		Document []byte `cbor:"document"`
	} `cbor:"Attestation,omitempty"`
	Error *string `cbor:"Error,omitempty"`
}

// encodeAttestationRequest does not check the nonce's size: the role adapter does, with its own Nonce.Validate,
// before calling Attest.
func encodeAttestationRequest(nonce, userData []byte) ([]byte, error) {
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
