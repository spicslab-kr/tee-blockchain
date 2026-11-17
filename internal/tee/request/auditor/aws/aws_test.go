package aws

import (
	"context"
	"path/filepath"
	"testing"

	"attestamp/internal/tee/request/auditor"
)

// A nonce of the wrong size is refused before any device is touched; the device here does not exist, so reaching it
// would give another error.
func TestExtract_RejectsShortNonce(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-nsm")
	if _, err := (Extractor{Device: missing}).Extract(context.Background(), auditor.Nonce("short")); err == nil ||
		err.Error() != auditor.Nonce("short").Validate().Error() {
		t.Fatalf("short nonce: got %v, want the nonce size error", err)
	}
}

func TestExtract_StopsOnDoneContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Extractor{}).Extract(ctx, make(auditor.Nonce, auditor.NonceSize)); err != context.Canceled {
		t.Fatalf("done context: got %v, want context.Canceled", err)
	}
}
