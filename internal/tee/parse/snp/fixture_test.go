package snp

import (
	_ "embed"
	"testing"
)

// Evidence recorded on a real GCP SEV-SNP guest (n2d-standard-2). See testdata/README.md.
var (
	//go:embed testdata/gcp-n2d-report.bin
	gcpReport []byte
	//go:embed testdata/gcp-n2d-auxblob.bin
	gcpCerts []byte
	//go:embed testdata/gcp-n2d-att.pb
	gcpVTPM []byte
)

// gcpNonce is the vTPM nonce: SHA-256 of "attestamp-snp-fixture-0001".
const gcpNonce = "d529879465187801d4b9ffc1c7d0848a2f028e5df9ab46d3eb7a9d5cdb7f925f"

// realDoc packs the recorded evidence the way the Vendor sends it.
func realDoc(t *testing.T) []byte {
	t.Helper()
	raw, err := Encode(gcpReport, gcpCerts, gcpVTPM)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
