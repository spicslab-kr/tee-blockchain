// Command pack joins one recorded SEV-SNP evidence pair into the document `att-user ra-parse` reads, with
// internal/tee/parse/snp.Encode. A recording (record/main.go, or the fixtures in this folder) keeps the SNP report, its
// certificate table and the vTPM attestation as three files; the responder sends them as one CBOR container, and
// ra-parse expects that container. Without this command the packing needed hand-written Go.
//
// Either name the three files, or give the common prefix of a fixture:
//
//	go run ./internal/tee/parse/snp/testdata/pack -report r.bin -certs auxblob.bin -vtpm att.pb -out doc.cbor
//	go run ./internal/tee/parse/snp/testdata/pack -fixture internal/tee/parse/snp/testdata/gcp-n2d-opt1p-v1 -out doc.cbor
//	att-user ra-parse --doc doc.cbor --format amd-sev-snp-initrd
//
// -fixture <p> reads <p>-report.bin, <p>-auxblob.bin and <p>-att.pb. The output is deterministic: the same three files
// always give the same bytes. It is under testdata so that `go build ./...` skips it.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"

	"attestamp/internal/tee/parse/snp"
)

func main() {
	report := flag.String("report", "", "the SNP attestation report (1184 bytes)")
	certs := flag.String("certs", "", "the certificate table (auxblob) returned with the report")
	vtpm := flag.String("vtpm", "", "the vTPM attestation, a go-tpm-tools attest.Attestation protobuf")
	fixture := flag.String("fixture", "", "instead of the three: the common prefix of <p>-report.bin, <p>-auxblob.bin, <p>-att.pb")
	out := flag.String("out", "", "where to write the packed document")
	flag.Parse()
	if err := run(*report, *certs, *vtpm, *fixture, *out); err != nil {
		fmt.Fprintln(os.Stderr, "pack:", err)
		os.Exit(1)
	}
}

func run(report, certs, vtpm, fixture, out string) error {
	if fixture != "" {
		if report != "" || certs != "" || vtpm != "" {
			return errors.New("give -fixture or the three files, not both")
		}
		report, certs, vtpm = fixture+"-report.bin", fixture+"-auxblob.bin", fixture+"-att.pb"
	}
	if report == "" || certs == "" || vtpm == "" || out == "" {
		return errors.New("-out and either -fixture or all of -report, -certs, -vtpm are required")
	}
	var parts [3][]byte
	for i, name := range []string{report, certs, vtpm} {
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		parts[i] = b
	}
	doc, err := snp.Encode(parts[0], parts[1], parts[2])
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, doc, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(doc)
	fmt.Printf("%s  %d bytes  sha256 %s\n", out, len(doc), hex.EncodeToString(sum[:]))
	return nil
}
