package azure

import (
	"context"
	"errors"

	"tb/internal/tee/request/auditor"
)

const Format = "azure-maa-jwt"

// Extractor — vTPM 에서 HCL 리포트를 읽어 MAA 에 보내고 JWT 를 받는다. go-tpm 이 필요해 아직 비어 있다.
type Extractor struct {
	MAAEndpoint string
}

var _ auditor.Extractor = (*Extractor)(nil)

func (Extractor) Extract(context.Context, auditor.Nonce) (auditor.RADoc, error) {
	return auditor.RADoc{}, errors.New("azure extractor not implemented — needs vTPM (go-tpm) and MAA")
}
