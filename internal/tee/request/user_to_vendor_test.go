// User requester to Vendor TEE extractor: checks that packages for different roles interlock over a single connection.
package request_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"

	vendor "attestamp/internal/tee/request/Vendor"
	user "attestamp/internal/tee/request/user"
)

type vendorTEE struct{ format string }

func (v vendorTEE) Extract(_ context.Context, n vendor.Nonce) (vendor.RADoc, error) {
	return vendor.RADoc{Format: v.format, Raw: append([]byte("attest:"), n...)}, nil
}

func serveVendor(t *testing.T, format string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = vendor.Serve(ctx, ln, vendorTEE{format}) }()
	return ln.Addr().String()
}

func TestRequest_UserToVendorTEE_AWS(t *testing.T) {
	addr := serveVendor(t, "aws-nitro")
	n, _ := user.NonceGen()
	doc, err := (user.TCPRequester{}).Request(context.Background(), n, addr)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.ExpectFormat("aws-nitro"); err != nil {
		t.Fatal(err)
	}
	if doc.Format != "aws-nitro" || !bytes.Equal(doc.Raw, append([]byte("attest:"), n...)) {
		t.Errorf("user got a different document: %+v", doc)
	}
}

func TestRequest_UserRejectsFormatMismatch(t *testing.T) {
	addr := serveVendor(t, "aws-nitro")
	n, _ := user.NonceGen()
	doc, err := (user.TCPRequester{}).Request(context.Background(), n, addr)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.ExpectFormat("gcp-cs-jwt"); !errors.Is(err, user.ErrWrongFormat) {
		t.Fatalf("expecting gcp must refuse an aws-nitro document, got %v", err)
	}
}
