// Package snp gets AMD SEV-SNP evidence from inside a GCP Confidential VM (formats amd-sev-snp-initrd and
// amd-sev-snp-verity). One piece of evidence is two documents: a vTPM attestation over the nonce, and an SNP report
// whose REPORT_DATA is SHA3-512 of that attestation, carried together in the CBOR container internal/tee/parse/snp
// reads. This is the sequence testdata/record/main.go records fixtures with.
//
// The logic is identical for every role, so it lives here once. Each role's internal/tee/request/<role>/snp is only a
// thin adapter that turns the evidence into that role's RADoc, the same two-layer shape as internal/tee/parse/snp.
//
// This package does not import internal/tee/parse/snp. att-ra-responder links it and runs inside the TEE, and the
// verifying side (go-sev-guest's verifier, go-tpm-tools' server) has no business in that binary or in its
// measurement. The container is therefore encoded here; snp_test.go checks it is byte-identical to parse/snp.Encode.
package snp

import (
	"bytes"
	"crypto/sha3"
	"errors"
	"fmt"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/go-configfs-tsm/configfs/configfsi"
	"github.com/google/go-configfs-tsm/configfs/linuxtsm"
	"github.com/google/go-configfs-tsm/report"
)

// The two format strings, the same as internal/tee/parse/snp's. The evidence is the same; the format decides which
// values are registered.
const (
	FormatInitrd = "amd-sev-snp-initrd"
	FormatVerity = "amd-sev-snp-verity"
)

// container is internal/tee/parse/snp's container: one CBOR map with text keys, in CBOR core deterministic encoding.
type container struct {
	Report []byte `cbor:"report"`
	Certs  []byte `cbor:"certs"`
	VTPM   []byte `cbor:"vtpm"`
}

var encMode, _ = cbor.CoreDetEncOptions().EncMode()

// encode packs the report, the host's certificate table and the vTPM attestation the way parse/snp.Encode does.
func encode(report, certs, vtpm []byte) ([]byte, error) {
	return encMode.Marshal(container{Report: report, Certs: certs, VTPM: vtpm})
}

// Provider is the ConfigFS TSM provider name of an SEV-SNP guest.
const Provider = "sev_guest"

// Attester produces the vTPM attestation over a nonce, as binary protobuf (go-tpm-tools attest.Attestation).
type Attester interface {
	Attest(nonce []byte) ([]byte, error)
}

// Client returns the ConfigFS TSM client of this machine. It fails where /sys/kernel/config/tsm does not exist.
func Client() (configfsi.Client, error) {
	c, err := linuxtsm.MakeClient()
	if err != nil {
		return nil, fmt.Errorf("snp: ConfigFS TSM: %w", err)
	}
	return c, nil
}

// Evidence makes one piece of evidence for the nonce:
//
//  1. the vTPM attestation over the nonce (its quotes carry the nonce as extraData);
//  2. REPORT_DATA = SHA3-512 of that attestation's bytes;
//  3. an SNP report over REPORT_DATA from ConfigFS TSM, with the host's certificate table (auxblob);
//  4. the three packed in the container internal/tee/parse/snp reads.
//
// It refuses a provider other than sev_guest and a report that does not carry the REPORT_DATA written.
func Evidence(client configfsi.Client, att Attester, nonce []byte) ([]byte, error) {
	if client == nil || att == nil {
		return nil, errors.New("snp: a ConfigFS TSM client and a vTPM attester are both required")
	}
	if len(nonce) == 0 {
		return nil, errors.New("snp: empty nonce")
	}
	vtpm, err := att.Attest(nonce)
	if err != nil {
		return nil, fmt.Errorf("snp: vTPM attestation: %w", err)
	}
	rd := sha3.Sum512(vtpm)
	resp, err := report.Get(client, &report.Request{InBlob: rd[:], GetAuxBlob: true})
	if err != nil {
		return nil, fmt.Errorf("snp: ConfigFS TSM report: %w", err)
	}
	if p := strings.TrimSpace(resp.Provider); p != Provider {
		return nil, fmt.Errorf("snp: ConfigFS TSM provider is %q, want %q (not an SEV-SNP guest?)", p, Provider)
	}
	r := resp.OutBlob
	if len(r) < 0x90 || !bytes.Equal(r[0x50:0x90], rd[:]) {
		return nil, fmt.Errorf("snp: the report (%d bytes) does not carry the REPORT_DATA written", len(r))
	}
	return encode(r, resp.AuxBlob, vtpm)
}
