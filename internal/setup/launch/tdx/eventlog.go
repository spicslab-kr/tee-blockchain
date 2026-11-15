package tdx

import (
	"bytes"
	"crypto/sha512"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-eventlog/register"
	"github.com/google/go-eventlog/tcg"
)

// ccMRRTMR2 is RTMR2's index in the CC event log: index 0 is MRTD, so RTMR<n> is n+1 (register.RTMR.Idx). GRUB's
// commands (TPM PCR 8) and the files it loads (PCR 9) both land in RTMR2 on a TDX guest.
const ccMRRTMR2 = 3

// grubBoot is what the replayed event log says GRUB did.
type grubBoot struct {
	initrdPath   string // the file the initrd command named
	initrdDigest []byte // SHA-384 of that file, as GRUB measured it
	cmdline      string // the kernel command line GRUB measured
}

// ErrEventLog: the event log does not replay to the quote's RTMRs, or does not say what GRUB loaded.
var ErrEventLog = errors.New("event log")

// grubFromLog replays the raw CC event log against RTMR0, RTMR1 and RTMR2 from a verified quote and reads GRUB's
// events from RTMR2. The log itself is not signed: only the replay ties it to the quote, so nothing is read from an
// event that did not replay. The same reading as the SEV-SNP parser's bootValues (internal/tee/parse/snp/measure.go),
// on the RTMR bank instead of the vTPM's PCRs.
func grubFromLog(raw []byte, rtmr [3][]byte) (grubBoot, error) {
	var g grubBoot
	bank := register.RTMRBank{RTMRs: []register.RTMR{{Index: 0, Digest: rtmr[0]}, {Index: 1, Digest: rtmr[1]}, {Index: 2, Digest: rtmr[2]}}}
	events, err := tcg.ParseAndReplay(raw, bank.MRs(), tcg.ParseOpts{AllowPadding: true}) // a CCEL is padded to its table size
	if err != nil {
		return g, fmt.Errorf("%w: does not replay to the quote's RTMR0-2: %v", ErrEventLog, err)
	}
	files := map[string][]byte{}
	var initrds int
	for _, e := range events {
		if e.MRIndex() != ccMRRTMR2 || e.UntrustedType() != tcg.Ipl {
			continue
		}
		data := strings.TrimRight(string(e.RawData()), "\x00")
		switch {
		case strings.HasPrefix(data, "grub_cmd: initrd "):
			if !textMeasured(strings.TrimPrefix(data, "grub_cmd: "), e.ReplayedDigest()) {
				return g, fmt.Errorf("%w: the initrd command's text does not match its measurement: %q", ErrEventLog, data)
			}
			initrds++
			paths := strings.Fields(strings.TrimPrefix(data, "grub_cmd: initrd "))
			if len(paths) != 1 {
				return g, fmt.Errorf("%w: GRUB loaded %d initrd files; one combined initrd is expected", ErrEventLog, len(paths))
			}
			g.initrdPath = paths[0]
		case strings.HasPrefix(data, "kernel_cmdline: "):
			if !textMeasured(strings.TrimPrefix(data, "kernel_cmdline: "), e.ReplayedDigest()) {
				return g, fmt.Errorf("%w: the kernel command line's text does not match its measurement", ErrEventLog)
			}
			g.cmdline = strings.TrimPrefix(data, "kernel_cmdline: ")
		case strings.HasPrefix(data, "grub_cmd: "), strings.HasPrefix(data, "grub_cmd "), strings.HasPrefix(data, "grub_kernel_cmdline "):
			// other commands
		default:
			files[data] = e.ReplayedDigest() // a file GRUB read; the last measurement under a name counts
		}
	}
	if initrds == 0 {
		return g, fmt.Errorf("%w: RTMR2 has no GRUB initrd command, so no initrd was loaded", ErrEventLog)
	}
	if initrds > 1 {
		return g, fmt.Errorf("%w: GRUB ran the initrd command %d times", ErrEventLog, initrds)
	}
	if g.initrdDigest = files[g.initrdPath]; len(g.initrdDigest) == 0 {
		return g, fmt.Errorf("%w: GRUB loaded %s but no measurement of it is in RTMR2", ErrEventLog, g.initrdPath)
	}
	if bytes.Count(g.initrdDigest, []byte{0}) == len(g.initrdDigest) {
		return g, fmt.Errorf("%w: the measurement of %s is all zero", ErrEventLog, g.initrdPath)
	}
	return g, nil
}

// textMeasured reports whether a GRUB string event's digest is the SHA-384 of its text. The replay vouches only for
// the digests; an event's text is not covered by it, so a command is believed only when its digest is the hash of the
// text after the prefix ("grub_cmd: ", "kernel_cmdline: "), with or without GRUB's terminating NUL. go-eventlog's
// GrubStateFromRTMRLog makes the same check. A file event's text, its name, cannot be checked this way; that is why
// the file's digest is compared with the built initrd rather than trusted by name alone.
func textMeasured(text string, digest []byte) bool {
	a := sha512.Sum384([]byte(text))
	b := sha512.Sum384([]byte(text + "\x00"))
	return bytes.Equal(digest, a[:]) || bytes.Equal(digest, b[:])
}
