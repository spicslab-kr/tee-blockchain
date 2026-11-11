package snp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-configfs-tsm/configfs/configfsi"
	"github.com/google/go-configfs-tsm/configfs/faketsm"

	parsesnp "attestamp/internal/tee/parse/snp"
)

type fixedAttester []byte

func (a fixedAttester) Attest([]byte) ([]byte, error) { return a, nil }

func fakeTSM(provider string, outblob func(inblob []byte) ([]byte, error), aux []byte) configfsi.Client {
	sub := &faketsm.ReportSubsystem{
		MakeEntry: func() *faketsm.ReportEntry {
			return &faketsm.ReportEntry{InAttrs: map[string]*faketsm.ReportAttributeState{"inblob": {}}}
		},
		CheckInAttr: func(_ *faketsm.ReportEntry, attr string, _ []byte) error {
			if attr != "inblob" {
				return errors.New("unexpected attribute " + attr)
			}
			return nil
		},
		ReadAttr: func(e *faketsm.ReportEntry, attr string) ([]byte, error) {
			switch attr {
			case "provider":
				return []byte(provider + "\n"), nil
			case "outblob":
				return outblob(e.InAttrs["inblob"].Value)
			case "auxblob":
				return aux, nil
			}
			return nil, os.ErrNotExist
		},
		Random: rand.Reader,
	}
	return &faketsm.Client{Subsystems: map[string]configfsi.Client{"report": sub}}
}

func recorded(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "parse", "snp", "testdata", "gcp-n2d-opt2-v1-"+name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The recording of 2026-11-11 (gcp-n2d-opt2-v1-*): a fake vTPM hands out its attestation, and a fake TSM hands out its
// report only when asked with SHA3-512 of exactly those bytes. So the evidence is produced only if Evidence computes the
// binding the guest and the parser use, and what it produces verifies as real amd-sev-snp-verity evidence.
func TestEvidence_RecordedEvidenceRoundTrip(t *testing.T) {
	att, rep, aux := recorded(t, "att.pb"), recorded(t, "report.bin"), recorded(t, "auxblob.bin")
	client := fakeTSM(Provider, func(in []byte) ([]byte, error) {
		if !bytes.Equal(in, rep[0x50:0x90]) {
			return nil, errors.New("not the REPORT_DATA the recording was made for")
		}
		return rep, nil
	}, aux)
	nonce := sha256.Sum256([]byte("attestamp-snp-opt2-v1"))
	raw, err := Evidence(client, fixedAttester(att), nonce[:])
	if err != nil {
		t.Fatal(err)
	}
	p := parsesnp.Parser{}
	if err := p.VerifySig(context.Background(), FormatVerity, raw, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.CheckNonce(FormatVerity, raw, nonce[:]); err != nil {
		t.Fatal(err)
	}
	if ms, err := p.Parse(FormatVerity, raw); err != nil || len(ms) != 6 {
		t.Fatalf("parse: %v %v", ms, err)
	}
}

// The container is encoded here, not by importing parse/snp (see the package comment), so the two encoders must agree
// byte for byte, and so must the format names.
func TestEncode_MatchesParseSNP(t *testing.T) {
	att, rep, aux := recorded(t, "att.pb"), recorded(t, "report.bin"), recorded(t, "auxblob.bin")
	for _, c := range [][3][]byte{{rep, aux, att}, {rep, nil, att}, {nil, nil, nil}} {
		ours, err := encode(c[0], c[1], c[2])
		if err != nil {
			t.Fatal(err)
		}
		theirs, err := parsesnp.Encode(c[0], c[1], c[2])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ours, theirs) {
			t.Fatalf("the container differs from parse/snp.Encode:\n%x\n%x", ours, theirs)
		}
	}
	if FormatInitrd != parsesnp.FormatInitrd || FormatVerity != parsesnp.FormatVerity {
		t.Fatal("the format names differ from parse/snp's")
	}
}

func TestEvidence_Refusals(t *testing.T) {
	att, rep, aux := recorded(t, "att.pb"), recorded(t, "report.bin"), recorded(t, "auxblob.bin")
	echo := func([]byte) ([]byte, error) { return rep, nil }
	nonce := []byte("n")
	if _, err := Evidence(fakeTSM("tdx_guest", echo, aux), fixedAttester(att), nonce); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Errorf("a non-SEV-SNP provider must be refused, got %v", err)
	}
	// a report bound to some other attestation
	if _, err := Evidence(fakeTSM(Provider, echo, aux), fixedAttester(append(att, 0)), nonce); err == nil || !strings.Contains(err.Error(), "REPORT_DATA") {
		t.Errorf("a report that does not carry the REPORT_DATA written must be refused, got %v", err)
	}
	if _, err := Evidence(nil, fixedAttester(att), nonce); err == nil {
		t.Error("no client must be an error")
	}
	if _, err := Evidence(fakeTSM(Provider, echo, aux), nil, nonce); err == nil {
		t.Error("no attester must be an error")
	}
}
