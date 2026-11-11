// Command record collects one Intel TDX quote on a GCP TDX guest, for use as a test fixture:
//
//	quote.bin   the quote, raw from ConfigFS TSM outblob, REPORTDATA = SHA-512 of -nonce-text (64 bytes)
//	record.txt  what was asked and what came back, and the boot it came from: nonce text, REPORTDATA, provider, quote
//	            size, the kernel command line and the boot ID
//
// A TDX quote is one document: there is no vTPM part, unlike internal/tee/parse/snp/testdata/record. The nonce
// convention is the one the first fixture used (testdata/README.md): REPORTDATA is SHA-512 of an ASCII string, so
// anyone can recompute it.
//
// It only runs on a TDX guest with ConfigFS TSM (/sys/kernel/config/tsm/report, provider tdx_guest; Linux 6.7 or
// later). It uses only the standard library. It is under testdata, so `go build ./...` skips it; build it with
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o record-tdx ./internal/tee/parse/tdx/testdata/record
//
// and run it as root:  record-tdx -out fx -nonce-text attestamp-tdx-app-v1
package main

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const tsmReportDir = "/sys/kernel/config/tsm/report"

func main() {
	out := flag.String("out", ".", "output directory")
	nonceText := flag.String("nonce-text", "", "REPORTDATA is SHA-512 of this ASCII string (64 bytes)")
	flag.Parse()
	if err := run(*out, *nonceText); err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		os.Exit(1)
	}
}

func run(out, nonceText string) error {
	if nonceText == "" {
		return errors.New("-nonce-text is required")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	rd := sha512.Sum512([]byte(nonceText))

	quote, provider, err := tsmQuote(rd[:])
	if err != nil {
		return fmt.Errorf("ConfigFS TSM: %w", err)
	}
	if err := os.WriteFile(filepath.Join(out, "quote.bin"), quote, 0o644); err != nil {
		return err
	}
	cmdline, _ := os.ReadFile("/proc/cmdline")
	bootID, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	info := fmt.Sprintf("nonce_text %s\nreport_data %s\nprovider %s\nquote_bytes %d\ncmdline %s\nboot_id %s\n",
		nonceText, hex.EncodeToString(rd[:]), provider, len(quote),
		strings.TrimSpace(string(cmdline)), strings.TrimSpace(string(bootID)))
	fmt.Print(info)
	return os.WriteFile(filepath.Join(out, "record.txt"), []byte(info), 0o644)
}

// tsmQuote asks the kernel for a quote through ConfigFS TSM: make an entry, check the provider, write REPORTDATA to
// inblob, read outblob, remove the entry. The quote body starts after the 48-byte header, and REPORTDATA is at body
// offset 520, which is checked so a quote for other data is never saved.
func tsmQuote(reportData []byte) (quote []byte, provider string, err error) {
	dir, err := os.MkdirTemp(tsmReportDir, "attestamp-")
	if err != nil {
		return nil, "", err
	}
	defer os.Remove(dir)
	p, err := os.ReadFile(filepath.Join(dir, "provider"))
	if err != nil {
		return nil, "", err
	}
	provider = strings.TrimSpace(string(p))
	if provider != "tdx_guest" {
		return nil, provider, fmt.Errorf("provider is %q, want tdx_guest", provider)
	}
	if err := os.WriteFile(filepath.Join(dir, "inblob"), reportData, 0o200); err != nil {
		return nil, provider, err
	}
	quote, err = os.ReadFile(filepath.Join(dir, "outblob"))
	if err != nil {
		return nil, provider, err
	}
	const offReportData = 48 + 520
	if len(quote) < offReportData+64 || !bytes.Equal(quote[offReportData:offReportData+64], reportData) {
		return nil, provider, fmt.Errorf("quote does not carry the REPORTDATA written (%d bytes)", len(quote))
	}
	return quote, provider, nil
}
