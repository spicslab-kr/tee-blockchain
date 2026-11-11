// Package tdx is the Vendor's adapter for getting an Intel TDX quote inside the guest. The work is done once in
// internal/tee/request/tdx; this only turns the quote into the Vendor's RADoc.
package tdx

import (
	"context"

	"github.com/google/go-configfs-tsm/configfs/configfsi"

	"attestamp/internal/tee/request/Vendor"
	core "attestamp/internal/tee/request/tdx"
)

const Format = core.Format

// Extractor asks ConfigFS TSM for a quote whose REPORTDATA is the nonce followed by zeros.
type Extractor struct {
	// Client is the ConfigFS TSM client; nil uses this machine's /sys/kernel/config/tsm.
	Client configfsi.Client
}

var _ vendor.Extractor = Extractor{}

func (e Extractor) Extract(ctx context.Context, nonce vendor.Nonce) (vendor.RADoc, error) {
	if err := nonce.Validate(); err != nil {
		return vendor.RADoc{}, err
	}
	if err := ctx.Err(); err != nil {
		return vendor.RADoc{}, err
	}
	client := e.Client
	if client == nil {
		var err error
		if client, err = core.Client(); err != nil {
			return vendor.RADoc{}, err
		}
	}
	q, err := core.Quote(client, nonce)
	if err != nil {
		return vendor.RADoc{}, err
	}
	return vendor.RADoc{Format: Format, Raw: q}, nil
}
