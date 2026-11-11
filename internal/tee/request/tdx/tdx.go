// Package tdx gets an Intel TDX quote from inside the guest (format intel-tdx), through the kernel's ConfigFS TSM
// interface (/sys/kernel/config/tsm/report, provider tdx_guest; Linux 6.7 or later). go-tdx-guest documents the older
// /dev/tdx_guest device as deprecated in favour of this interface.
//
// The logic is identical for every role, so it lives here once. Each role's internal/tee/request/<role>/tdx is only a
// thin adapter that turns the quote into that role's RADoc, the same two-layer shape as internal/tee/parse/tdx.
package tdx

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-configfs-tsm/configfs/configfsi"
	"github.com/google/go-configfs-tsm/configfs/linuxtsm"
	"github.com/google/go-configfs-tsm/report"
)

// Format is the format string the quote is labelled with; internal/tee/parse/tdx opens it.
const Format = "intel-tdx"

// Provider is the ConfigFS TSM provider name of a TDX guest.
const Provider = "tdx_guest"

// ReportDataSize is the size of REPORTDATA in the TD report.
const ReportDataSize = 64

// offReportData is where REPORTDATA sits in a version 4 quote: after the 48-byte header, at offset 520 of the TD quote
// body. It is used only to check that the quote returned carries the REPORTDATA written.
const offReportData = 48 + 520

// ReportData is what the quote's REPORTDATA is set to for a nonce: the nonce, then zeros up to 64 bytes. That is the
// layout internal/tee/parse/tdx's CheckNonce expects. A tee/request nonce is 32 bytes; a 64-byte nonce fills all of it.
func ReportData(nonce []byte) ([]byte, error) {
	if len(nonce) == 0 || len(nonce) > ReportDataSize {
		return nil, fmt.Errorf("tdx: nonce is %d bytes, REPORTDATA holds 1 to %d", len(nonce), ReportDataSize)
	}
	rd := make([]byte, ReportDataSize)
	copy(rd, nonce)
	return rd, nil
}

// Client returns the ConfigFS TSM client of this machine. It fails where /sys/kernel/config/tsm does not exist.
func Client() (configfsi.Client, error) {
	c, err := linuxtsm.MakeClient()
	if err != nil {
		return nil, fmt.Errorf("tdx: ConfigFS TSM: %w", err)
	}
	return c, nil
}

// Quote asks the TSM for a quote over the nonce and returns it as is. It refuses a provider other than tdx_guest and a
// quote that does not carry the REPORTDATA written, so a quote for other data is never handed out.
func Quote(client configfsi.Client, nonce []byte) ([]byte, error) {
	if client == nil {
		return nil, errors.New("tdx: no ConfigFS TSM client")
	}
	rd, err := ReportData(nonce)
	if err != nil {
		return nil, err
	}
	resp, err := report.Get(client, &report.Request{InBlob: rd})
	if err != nil {
		return nil, fmt.Errorf("tdx: ConfigFS TSM report: %w", err)
	}
	if p := strings.TrimSpace(resp.Provider); p != Provider {
		return nil, fmt.Errorf("tdx: ConfigFS TSM provider is %q, want %q (not a TDX guest?)", p, Provider)
	}
	q := resp.OutBlob
	if len(q) < offReportData+ReportDataSize || !bytes.Equal(q[offReportData:offReportData+ReportDataSize], rd) {
		return nil, fmt.Errorf("tdx: the quote (%d bytes) does not carry the REPORTDATA written", len(q))
	}
	return q, nil
}
