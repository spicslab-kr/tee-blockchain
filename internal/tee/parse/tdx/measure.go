package tdx

import (
	"fmt"
	"time"

	pb "github.com/google/go-tdx-guest/proto/tdx"
)

type Measurement struct {
	Name  string
	Value []byte
}

type Measurements []Measurement

// Get finds a value by name, or returns nil.
func (ms Measurements) Get(name string) []byte {
	for _, m := range ms {
		if m.Name == name {
			return m.Value
		}
	}
	return nil
}

// Info is what the quote says besides the measurements: nonce and debug mode. A TDX quote has no timestamp and no
// instance identity, so Timestamp is zero and Module is empty.
type Info struct {
	Format    string
	Timestamp time.Time // always zero: a TDX quote carries no time
	Nonce     []byte    // the 64 bytes of REPORTDATA
	UserData  []byte
	Debug     bool // debug-mode TD: the host can read and change its memory, so the measurements are meaningless
	Module    string
}

// imageRTMRs are the runtime registers that describe the boot chain. A UEFI TD guest's firmware extends what a TPM
// would put in PCR1 and 7 into RTMR0, PCR2-6 into RTMR1 and PCR8-15 into RTMR2, so RTMR0 holds the virtual firmware's
// configuration, RTMR1 the boot loader and kernel, and RTMR2 the kernel command line and the initrd (where the
// application is). That mapping is the TDVF design's; on GCP, RTMR2 moved with the initrd and RTMR0 with the machine
// type, and nothing else did (2026-11-11 and -16), while a kernel change moving RTMR1 was not measured; see
// docs/build/tdx.md and docs/status/test-matrix.md, T1. Together with MRTD (the virtual firmware) they are registered.
//
// MRTD alone does not do: it is the same for every guest on the same firmware. RTMR2 alone does not do either: the
// firmware, boot loader and kernel extend RTMR2 themselves, so a changed kernel could extend the expected RTMR2 while
// running something else, and only MRTD and RTMR0-1 would show it. This is the same reason Nitro registers PCR0, 1 and
// 2 together.
//
// Left out because they do not describe the image: RTMR3 (extended at run time by the guest, zero on a fresh boot),
// MROWNER, MROWNERCONFIG and MRCONFIGID (set by whoever launches the TD; MROWNER is assumed to differ per GCP instance, not measured), and the
// TDX module's MRSEAM (the platform, not the guest). Nitro leaves out PCR3, PCR4 and PCR8 for the same reason.
var imageRTMRs = []int{0, 1, 2}

// tdAttributesDebug is bit 0 of TDATTRIBUTES, the TUD.DEBUG bit of Intel's TDX module specification. In a debug TD the
// host VMM can read and write the TD's private memory and CPU state.
const tdAttributesDebug = 0x01

// Parse returns MRTD and RTMR0, 1 and 2 under the names MRTD and RTMR<n>. A debug-mode TD is refused: its values could
// have been changed by the host after they were measured. Nitro refuses the same case because PCR0 is then all zero.
func (a Parser) Parse(format string, raw []byte) (Measurements, error) {
	q, err := decode(format, raw)
	if err != nil {
		return nil, err
	}
	body := q.GetTdQuoteBody()
	if isDebug(body) {
		return nil, fmt.Errorf("%w: the TD runs in debug mode (TDATTRIBUTES.DEBUG is set)", ErrBadDocument)
	}
	out := Measurements{{Name: "MRTD", Value: body.GetMrTd()}}
	for _, i := range imageRTMRs {
		out = append(out, Measurement{Name: fmt.Sprintf("RTMR%d", i), Value: body.GetRtmrs()[i]})
	}
	for _, m := range out {
		if len(m.Value) != measurementSize {
			return nil, fmt.Errorf("%w: %s has %d bytes, want %d", ErrBadDocument, m.Name, len(m.Value), measurementSize)
		}
	}
	return out, nil
}

// Info returns REPORTDATA as the nonce and the debug flag.
func (a Parser) Info(format string, raw []byte) (Info, error) {
	q, err := decode(format, raw)
	if err != nil {
		return Info{}, err
	}
	body := q.GetTdQuoteBody()
	return Info{
		Format: Format,
		Nonce:  body.GetReportData(),
		Debug:  isDebug(body),
	}, nil
}

// isDebug reads TDATTRIBUTES, which the quote stores little endian: bit 0 is in the first byte.
func isDebug(body *pb.TDQuoteBody) bool {
	attr := body.GetTdAttributes()
	return len(attr) > 0 && attr[0]&tdAttributesDebug != 0
}
