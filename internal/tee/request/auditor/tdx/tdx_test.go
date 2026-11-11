package tdx

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"testing"

	"github.com/google/go-configfs-tsm/configfs/configfsi"
	"github.com/google/go-configfs-tsm/configfs/faketsm"

	"attestamp/internal/tee/request/auditor"
)

// echoTSM returns a quote-shaped blob carrying the REPORTDATA written, as a TDX guest's TSM does.
func echoTSM() configfsi.Client {
	sub := &faketsm.ReportSubsystem{
		MakeEntry: func() *faketsm.ReportEntry {
			return &faketsm.ReportEntry{InAttrs: map[string]*faketsm.ReportAttributeState{"inblob": {}}}
		},
		CheckInAttr: func(*faketsm.ReportEntry, string, []byte) error { return nil },
		ReadAttr: func(e *faketsm.ReportEntry, attr string) ([]byte, error) {
			switch attr {
			case "provider":
				return []byte("tdx_guest\n"), nil
			case "outblob":
				q := make([]byte, 48+520+64)
				copy(q[48+520:], e.InAttrs["inblob"].Value)
				return q, nil
			}
			return nil, os.ErrNotExist
		},
		Random: rand.Reader,
	}
	return &faketsm.Client{Subsystems: map[string]configfsi.Client{"report": sub}}
}

func TestExtract(t *testing.T) {
	nonce := auditor.Nonce(bytes.Repeat([]byte{7}, auditor.NonceSize))
	doc, err := Extractor{Client: echoTSM()}.Extract(context.Background(), nonce)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != "intel-tdx" || !bytes.Equal(doc.Raw[48+520:48+520+32], nonce) {
		t.Fatalf("doc = %s, REPORTDATA %x", doc.Format, doc.Raw[48+520:])
	}
	if _, err := (Extractor{Client: echoTSM()}).Extract(context.Background(), auditor.Nonce{1, 2}); err == nil {
		t.Error("a nonce of the wrong size must be refused")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Extractor{Client: echoTSM()}).Extract(ctx, nonce); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled request must not ask the TSM, got %v", err)
	}
}
