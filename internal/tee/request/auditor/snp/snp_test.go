package snp

import (
	"bytes"
	"context"
	"testing"

	"attestamp/internal/tee/request/auditor"
)

// The end-to-end path is tested on recorded evidence in internal/tee/request/snp. Here: the adapter's own checks.
func TestExtract_Checks(t *testing.T) {
	nonce := auditor.Nonce(bytes.Repeat([]byte{7}, auditor.NonceSize))
	if _, err := (Extractor{Format: "amd-sev-snp"}).Extract(context.Background(), nonce); err == nil {
		t.Error("a format other than the two SEV-SNP formats must be refused")
	}
	if _, err := (Extractor{Format: FormatInitrd}).Extract(context.Background(), auditor.Nonce{1}); err == nil {
		t.Error("a nonce of the wrong size must be refused")
	}
}
