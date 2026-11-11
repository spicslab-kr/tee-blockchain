// Package snp is the Auditor's adapter for getting AMD SEV-SNP evidence inside a GCP guest. The work is done once in
// internal/tee/request/snp; this only turns the evidence into the Auditor's RADoc.
package snp

import (
	"context"
	"fmt"

	"github.com/google/go-configfs-tsm/configfs/configfsi"

	"attestamp/internal/tee/request/auditor"
	core "attestamp/internal/tee/request/snp"
)

const (
	FormatInitrd = core.FormatInitrd
	FormatVerity = core.FormatVerity
)

// Extractor makes the vTPM attestation over the nonce, binds it into an SNP report by SHA3-512 and returns both as one
// document, labelled with Format.
type Extractor struct {
	// Format is FormatInitrd or FormatVerity: the same evidence, registered with different value lists.
	Format string
	// Client is the ConfigFS TSM client; nil uses this machine's /sys/kernel/config/tsm.
	Client configfsi.Client
	// Attester makes the vTPM attestation; nil uses the GCE vTPM (/dev/tpmrm0).
	Attester core.Attester
}

var _ auditor.Extractor = Extractor{}

func (e Extractor) Extract(ctx context.Context, nonce auditor.Nonce) (auditor.RADoc, error) {
	if e.Format != FormatInitrd && e.Format != FormatVerity {
		return auditor.RADoc{}, fmt.Errorf("snp: format %q, want %s or %s", e.Format, FormatInitrd, FormatVerity)
	}
	if err := nonce.Validate(); err != nil {
		return auditor.RADoc{}, err
	}
	if err := ctx.Err(); err != nil {
		return auditor.RADoc{}, err
	}
	client := e.Client
	if client == nil {
		var err error
		if client, err = core.Client(); err != nil {
			return auditor.RADoc{}, err
		}
	}
	att := e.Attester
	if att == nil {
		att = core.GCEAttester{}
	}
	raw, err := core.Evidence(client, att, nonce)
	if err != nil {
		return auditor.RADoc{}, err
	}
	return auditor.RADoc{Format: e.Format, Raw: raw}, nil
}
