package tdx

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha512"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-configfs-tsm/configfs/configfsi"
	"github.com/google/go-configfs-tsm/configfs/faketsm"

	parsetdx "attestamp/internal/tee/parse/tdx"
)

// fakeTSM is a ConfigFS TSM report subsystem that answers with outblob(inblob) and the given provider, and records the
// last inblob written.
func fakeTSM(provider string, outblob func(inblob []byte) ([]byte, error), written *[]byte) configfsi.Client {
	sub := &faketsm.ReportSubsystem{
		MakeEntry: func() *faketsm.ReportEntry {
			return &faketsm.ReportEntry{InAttrs: map[string]*faketsm.ReportAttributeState{"inblob": {}}}
		},
		CheckInAttr: func(_ *faketsm.ReportEntry, attr string, contents []byte) error {
			if attr != "inblob" {
				return errors.New("unexpected attribute " + attr)
			}
			if written != nil {
				*written = append([]byte(nil), contents...)
			}
			return nil
		},
		ReadAttr: func(e *faketsm.ReportEntry, attr string) ([]byte, error) {
			switch attr {
			case "provider":
				return []byte(provider + "\n"), nil
			case "outblob":
				return outblob(e.InAttrs["inblob"].Value)
			}
			return nil, os.ErrNotExist
		},
		Random: rand.Reader,
	}
	return &faketsm.Client{Subsystems: map[string]configfsi.Client{"report": sub}}
}

// The recorded quote of 2026-11-11 (internal/tee/parse/tdx/testdata/gcp-c3-opt1-v1-quote.bin) was made for REPORTDATA =
// SHA-512 of "attestamp-tdx-opt1-v1". A fake TSM that returns it only for that REPORTDATA shows Quote writes the nonce
// where the guest expects it, and the quote it hands out verifies with the parser.
func TestQuote_RecordedQuoteRoundTrip(t *testing.T) {
	recorded, err := os.ReadFile(filepath.Join("..", "..", "parse", "tdx", "testdata", "gcp-c3-opt1-v1-quote.bin"))
	if err != nil {
		t.Fatal(err)
	}
	nonce := sha512.Sum512([]byte("attestamp-tdx-opt1-v1"))
	client := fakeTSM(Provider, func(in []byte) ([]byte, error) {
		if !bytes.Equal(in, nonce[:]) {
			return nil, errors.New("not the REPORTDATA the recording was made for")
		}
		return recorded, nil
	}, nil)
	q, err := Quote(client, nonce[:])
	if err != nil {
		t.Fatal(err)
	}
	p := parsetdx.Parser{}
	if err := p.VerifySig(context.Background(), parsetdx.Format, q, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.CheckNonce(parsetdx.Format, q, nonce[:]); err != nil {
		t.Fatal(err)
	}
}

// A tee/request nonce is 32 bytes: REPORTDATA is the nonce, then 32 zero bytes, which is what parse/tdx's CheckNonce
// accepts for a 32-byte nonce.
func TestQuote_ThirtyTwoByteNonceIsZeroPadded(t *testing.T) {
	nonce := bytes.Repeat([]byte{0xA5}, 32)
	var written []byte
	client := fakeTSM(Provider, func(in []byte) ([]byte, error) {
		q := make([]byte, offReportData+ReportDataSize+16)
		copy(q[offReportData:], in)
		return q, nil
	}, &written)
	if _, err := Quote(client, nonce); err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte(nil), nonce...), make([]byte, 32)...)
	if !bytes.Equal(written, want) {
		t.Fatalf("inblob = %x, want the nonce then 32 zero bytes", written)
	}
}

func TestQuote_Refusals(t *testing.T) {
	nonce := bytes.Repeat([]byte{1}, 32)
	echo := func(in []byte) ([]byte, error) {
		q := make([]byte, offReportData+ReportDataSize)
		copy(q[offReportData:], in)
		return q, nil
	}
	if _, err := Quote(fakeTSM("sev_guest", echo, nil), nonce); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Errorf("a non-TDX provider must be refused, got %v", err)
	}
	other := func([]byte) ([]byte, error) { return make([]byte, offReportData+ReportDataSize), nil }
	if _, err := Quote(fakeTSM(Provider, other, nil), nonce); err == nil || !strings.Contains(err.Error(), "REPORTDATA") {
		t.Errorf("a quote for other REPORTDATA must be refused, got %v", err)
	}
	if _, err := Quote(fakeTSM(Provider, echo, nil), make([]byte, 65)); err == nil {
		t.Error("a nonce longer than REPORTDATA must be refused")
	}
	if _, err := Quote(nil, nonce); err == nil {
		t.Error("no client must be an error")
	}
}
