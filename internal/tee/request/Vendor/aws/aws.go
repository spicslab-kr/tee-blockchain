// Package aws is the Vendor's adapter for AWS Nitro. The work is done once in internal/tee/request/aws (the NSM
// request, vsock, the proxy, the enclave's loopback); this checks the nonce, turns the document into the Vendor's
// RADoc, and passes the rest through, as internal/tee/request/<role>/tdx does over internal/tee/request/tdx.
// The Vendor and auditor copies are the same apart from the role (STRUCTURE.md, "Why role code is copied per role").
package aws

import (
	"context"
	"net"

	"attestamp/internal/tee/request/Vendor"
	core "attestamp/internal/tee/request/aws"
)

// Format is the document format name of Nitro attestation documents.
const Format = core.Format

// Extractor requests a document from /dev/nsm, from inside the enclave.
type Extractor struct {
	Device   string // default /dev/nsm
	UserData []byte // value to bind into the document; nil if none
}

var _ vendor.Extractor = (*Extractor)(nil)

func (e Extractor) Extract(ctx context.Context, nonce vendor.Nonce) (vendor.RADoc, error) {
	if err := ctx.Err(); err != nil {
		return vendor.RADoc{}, err
	}
	if err := nonce.Validate(); err != nil {
		return vendor.RADoc{}, err
	}
	doc, err := core.Attest(ctx, e.Device, nonce, e.UserData)
	if err != nil {
		return vendor.RADoc{}, err
	}
	return vendor.RADoc{Format: Format, Raw: doc}, nil
}

// ListenVsock listens on a vsock port inside the enclave; see internal/tee/request/aws.
func ListenVsock(port uint32) (net.Listener, error) { return core.ListenVsock(port) }

// LoopbackUp brings up the enclave's loopback interface; see internal/tee/request/aws.
func LoopbackUp() error { return core.LoopbackUp() }

// Proxy bridges TCP connections on ln to the enclave's vsock port, on the parent instance; see
// internal/tee/request/aws.
func Proxy(ctx context.Context, ln net.Listener, cid, port uint32) error {
	return core.Proxy(ctx, ln, cid, port)
}
