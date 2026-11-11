// Package tdx is the Auditor's adapter for getting an Intel TDX quote inside the guest. The work is done once in
// internal/tee/request/tdx; this only turns the quote into the Auditor's RADoc.
package tdx

import (
	"context"

	"github.com/google/go-configfs-tsm/configfs/configfsi"

	"attestamp/internal/tee/request/auditor"
	core "attestamp/internal/tee/request/tdx"
)

const Format = core.Format

// Extractor asks ConfigFS TSM for a quote whose REPORTDATA is the nonce followed by zeros.
type Extractor struct {
	// Client is the ConfigFS TSM client; nil uses this machine's /sys/kernel/config/tsm.
	Client configfsi.Client
}

var _ auditor.Extractor = Extractor{}

func (e Extractor) Extract(ctx context.Context, nonce auditor.Nonce) (auditor.RADoc, error) {
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
	q, err := core.Quote(client, nonce)
	if err != nil {
		return auditor.RADoc{}, err
	}
	return auditor.RADoc{Format: Format, Raw: q}, nil
}
