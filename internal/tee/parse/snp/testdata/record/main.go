// Command record collects one SEV-SNP evidence pair on a GCP Confidential VM, for use as a test fixture:
//
//	att.pb      a vTPM attestation (go-tpm-tools attest.Attestation, binary protobuf) over a fixed nonce, signed by
//	            the GCE attestation key (RSA), with the AK certificate, its intermediates and the TCG event log
//	report.bin  the SNP attestation report, REPORT_DATA = SHA3-512(att.pb), read through ConfigFS TSM
//	auxblob.bin the certificate table the host returned with the report, if it returned one
//
// The binding is REPORT_DATA = SHA3-512 of the vTPM attestation, as internal/tee/parse/snp checks it. It was SHA-512
// until 2026-11-11 (the hash the 2026-11 SEV-SNP scripts used; they are no longer available); the
// gcp-n2d-initrdless-* fixture was recorded with that older binding.
//
// Run as root:  record -out <dir> -nonce-text attestamp-snp-fixture-0001
//
// For test-matrix row T10 (docs/build/app-fixture-recording.md, "Reproducing T10"), -relay-att <att.pb from another VM>
// makes no vTPM attestation here: it copies that file to att.pb and requests this VM's SNP report over SHA3-512 of it.
// The result is the relay: another VM's vTPM evidence, bound by this VM's AMD-signed report. record.txt says so.
package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha3"
	"encoding/hex"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-tpm-tools/client"
	"github.com/google/go-tpm/legacy/tpm2"
	"google.golang.org/protobuf/proto"
)

func main() {
	out := flag.String("out", ".", "output directory")
	nonceText := flag.String("nonce-text", "attestamp-snp-fixture-0001", "the vTPM nonce is SHA-256 of this ASCII string (32 bytes, the tee/request nonce size)")
	relayAtt := flag.String("relay-att", "", "T10 only: an att.pb recorded on another VM. no vTPM attestation is made here; this VM's report is requested over SHA3-512 of that file")
	flag.Parse()
	if err := run(*out, *nonceText, *relayAtt); err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		os.Exit(1)
	}
}

func run(out, nonceText, relayAtt string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	n := sha256.Sum256([]byte(nonceText))
	nonce := n[:]

	var attRaw []byte
	var err error
	if relayAtt != "" {
		// T10: the vTPM evidence is another VM's, unchanged; only the report is this VM's.
		if attRaw, err = os.ReadFile(relayAtt); err != nil {
			return fmt.Errorf("relay attestation: %w", err)
		}
	} else if attRaw, err = attest(nonce); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "att.pb"), attRaw, 0o644); err != nil {
		return err
	}
	rd := sha3.Sum512(attRaw)

	report, aux, provider, err := tsmReport(rd[:])
	if err != nil {
		return fmt.Errorf("ConfigFS TSM: %w", err)
	}
	if err := os.WriteFile(filepath.Join(out, "report.bin"), report, 0o644); err != nil {
		return err
	}
	if len(aux) > 0 {
		if err := os.WriteFile(filepath.Join(out, "auxblob.bin"), aux, 0o644); err != nil {
			return err
		}
	}
	info := fmt.Sprintf("nonce_text %s\nnonce %s\nreport_data %s\nprovider %s\natt_pb_bytes %d\nreport_bytes %d\nauxblob_bytes %d\n",
		nonceText, hex.EncodeToString(nonce), hex.EncodeToString(rd[:]), provider, len(attRaw), len(report), len(aux))
	if relayAtt != "" {
		info += fmt.Sprintf("relay_att %s (another VM's vTPM attestation; the report is this VM's)\n", filepath.Base(relayAtt))
	}
	fmt.Print(info)
	return os.WriteFile(filepath.Join(out, "record.txt"), []byte(info), 0o644)
}

// attest makes a vTPM attestation over nonce with the GCE attestation key and returns it as binary protobuf.
func attest(nonce []byte) ([]byte, error) {
	rwc, err := tpm2.OpenTPM("/dev/tpmrm0")
	if err != nil {
		return nil, fmt.Errorf("open TPM: %w", err)
	}
	defer rwc.Close()
	ak, err := client.GceAttestationKeyRSA(rwc)
	if err != nil {
		return nil, fmt.Errorf("GCE AK: %w", err)
	}
	defer ak.Close()
	att, err := ak.Attest(client.AttestOpts{Nonce: nonce, CertChainFetcher: http.DefaultClient})
	if err != nil {
		return nil, fmt.Errorf("attest: %w", err)
	}
	return proto.Marshal(att)
}

// tsmReport asks the kernel for a report through /sys/kernel/config/tsm/report: make an entry, write REPORT_DATA to
// inblob, read outblob (and auxblob, the host's certificate table), remove the entry.
func tsmReport(reportData []byte) (report, aux []byte, provider string, err error) {
	dir, err := os.MkdirTemp("/sys/kernel/config/tsm/report", "attestamp-")
	if err != nil {
		return nil, nil, "", err
	}
	defer os.Remove(dir)
	p, err := os.ReadFile(filepath.Join(dir, "provider"))
	if err != nil {
		return nil, nil, "", err
	}
	provider = strings.TrimSpace(string(p))
	if provider != "sev_guest" {
		return nil, nil, provider, fmt.Errorf("provider is %q, want sev_guest", provider)
	}
	if err := os.WriteFile(filepath.Join(dir, "inblob"), reportData, 0o200); err != nil {
		return nil, nil, provider, err
	}
	report, err = os.ReadFile(filepath.Join(dir, "outblob"))
	if err != nil {
		return nil, nil, provider, err
	}
	aux, _ = os.ReadFile(filepath.Join(dir, "auxblob")) // absent when the host returns no certificates
	if len(report) < 0x90 || !bytes.Equal(report[0x50:0x90], reportData) {
		return nil, nil, provider, fmt.Errorf("report does not carry the REPORT_DATA written (%d bytes)", len(report))
	}
	return report, aux, provider, nil
}
