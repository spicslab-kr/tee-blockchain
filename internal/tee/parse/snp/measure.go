package snp

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	sabi "github.com/google/go-sev-guest/abi"
	attest "github.com/google/go-tpm-tools/proto/attest"
	tpmpb "github.com/google/go-tpm-tools/proto/tpm"
	"github.com/google/go-tpm-tools/server"
	"github.com/google/go-tpm/legacy/tpm2"
	"google.golang.org/protobuf/proto"
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

// Info is what the evidence says besides the measurements: the nonce and debug mode. Neither document carries a time
// or an identity this project uses, so Timestamp is zero and Module is empty.
type Info struct {
	Format    string
	Timestamp time.Time // always zero: neither document carries a time
	Nonce     []byte    // the vTPM quotes' extraData
	UserData  []byte
	Debug     bool // POLICY.DEBUG: the host may debug the guest, so the measurements are meaningless
	Module    string
}

// The names registered on chain. Every value is what the evidence shows, never a value recomputed from a guess:
//
//	MEASUREMENT  the SNP launch digest (48 bytes, from the AMD-signed report): the virtual firmware the guest started in
//	PCR4         vTPM PCR4 (SHA-256 bank): the boot applications the firmware ran, shim and GRUB on GCP's images
//	KERNEL       SHA-256 of the kernel file GRUB loaded, as GRUB measured it into PCR9
//	CMDLINE      SHA-256 of the kernel command line GRUB passed (its kernel_cmdline event, as text)
//	INITRD       SHA-256 of the initrd file GRUB loaded, as GRUB measured it into PCR9
//	PCR12        vTPM PCR12 (SHA-256 bank), verity only: the dm-verity result the verifying initrd extended
//
// INITRD alone would not do: GRUB and the kernel are what measure and load it, so a changed boot loader or kernel
// could report the expected initrd while booting another. PCR4, KERNEL and CMDLINE close that. The kernel command line
// carries the root partition's PARTUUID, so the Auditor uses the command line the Vendor publishes (as in the 2026-11
// runs) rather than one of its own.
//
// The file names in GRUB's events are not covered by the digests ("untrusted" in go-tpm-tools). They are used only to
// tell the kernel and the initrd apart; GRUB, which writes them, is itself covered by PCR4.
//
// The SHA-256 bank is used because the verity hook extends only that bank, and because the 2026-11 Auditor scripts
// computed their expected values with SHA-256.
var (
	initrdNames = []string{"MEASUREMENT", "PCR4", "KERNEL", "CMDLINE", "INITRD"}
	verityNames = []string{"MEASUREMENT", "PCR4", "KERNEL", "CMDLINE", "INITRD", "PCR12"}
)

// MeasurementNames returns the names a format registers, in the order Parse returns them.
func MeasurementNames(format string) []string {
	if format == FormatVerity {
		return append([]string(nil), verityNames...)
	}
	return append([]string(nil), initrdNames...)
}

// verityRootHashArg is the kernel command line argument the verifying initrd reads its root hash from.
const verityRootHashArg = "attestamp.roothash="

// Parse returns the values for the format (see the names above). It refuses a debug guest, an initrd format whose
// guest booted without an initrd, and a verity format whose command line has no root hash or whose PCR12 was never
// extended. Call it after VerifySig succeeds: it re-reads the event log but does not check the AK's chain.
func (a Parser) Parse(format string, raw []byte) (Measurements, error) {
	ev, err := decode(format, raw)
	if err != nil {
		return nil, err
	}
	if format == "" {
		return nil, fmt.Errorf("%w: give %s or %s; the evidence is the same, the values registered are not", ErrWrongFormat, FormatInitrd, FormatVerity)
	}
	if isDebug(ev) {
		return nil, fmt.Errorf("%w: the guest policy allows debugging (POLICY.DEBUG is set)", ErrBadDocument)
	}
	b, err := bootValues(ev)
	if err != nil {
		return nil, err
	}
	out := Measurements{
		{Name: "MEASUREMENT", Value: ev.report.GetReport().GetMeasurement()},
		{Name: "PCR4", Value: b.pcrs[4]},
		{Name: "KERNEL", Value: b.kernel},
		{Name: "CMDLINE", Value: b.cmdline},
	}
	if b.initrd == nil {
		return nil, fmt.Errorf("%w: GRUB loaded no initrd, so the application archive was never measured (an initrdless boot; see docs/build/sev-snp.md)", ErrBadDocument)
	}
	out = append(out, Measurement{Name: "INITRD", Value: b.initrd})
	if format == FormatVerity {
		if !strings.Contains(b.cmdlineText, verityRootHashArg) {
			return nil, fmt.Errorf("%w: the kernel command line has no %s, so no disk image was checked", ErrBadDocument, verityRootHashArg)
		}
		if isZero(b.pcrs[12]) {
			return nil, fmt.Errorf("%w: PCR12 was never extended, so the dm-verity check did not report", ErrBadDocument)
		}
		out = append(out, Measurement{Name: "PCR12", Value: b.pcrs[12]})
	}
	return out, nil
}

// Info returns the quotes' nonce and the debug flag.
func (a Parser) Info(format string, raw []byte) (Info, error) {
	ev, err := decode(format, raw)
	if err != nil {
		return Info{}, err
	}
	extra, err := quoteExtraData(ev)
	if err != nil {
		return Info{}, err
	}
	return Info{Format: format, Nonce: extra, Debug: isDebug(ev)}, nil
}

func isDebug(ev evidence) bool {
	p, err := sabi.ParseSnpPolicy(ev.report.GetReport().GetPolicy())
	return err != nil || p.Debug // a policy that does not parse is treated as untrusted
}

// boot holds what the vTPM evidence says about the boot.
type boot struct {
	pcrs                    map[uint32][]byte // SHA-256 bank
	kernel, initrd, cmdline []byte
	cmdlineText             string
}

// bootValues replays the event log against the SHA-256 quote with go-tpm-tools and reads GRUB's events.
func bootValues(ev evidence) (boot, error) {
	var b boot
	ak, err := x509.ParseCertificate(ev.vtpm.GetAkCert())
	if err != nil {
		return b, fmt.Errorf("%w: AK certificate: %v", ErrBadDocument, err)
	}
	extra, err := quoteExtraData(ev)
	if err != nil {
		return b, err
	}
	ms, err := server.VerifyAttestation(proto.Clone(ev.vtpm).(*attest.Attestation), server.VerifyOpts{
		Nonce: extra, TrustedAKs: []crypto.PublicKey{ak.PublicKey}, Loader: server.GRUB, HashAlgo: tpm2.AlgSHA256,
	})
	if err != nil {
		return b, fmt.Errorf("%w: vTPM event log: %v", ErrSignature, err)
	}
	for _, q := range ev.vtpm.GetQuotes() {
		if q.GetPcrs().GetHash() == tpmpb.HashAlgo_SHA256 {
			b.pcrs = q.GetPcrs().GetPcrs()
		}
	}
	if b.pcrs == nil {
		return b, fmt.Errorf("%w: no SHA-256 quote", ErrBadDocument)
	}

	var kernelPath, initrdPath string
	for _, c := range ms.GetGrub().GetCommands() {
		c = strings.TrimRight(c, "\x00")
		switch {
		case strings.HasPrefix(c, "grub_cmd: linux "):
			kernelPath = firstField(strings.TrimPrefix(c, "grub_cmd: linux "))
		case strings.HasPrefix(c, "grub_cmd: initrd "):
			paths := strings.Fields(strings.TrimPrefix(c, "grub_cmd: initrd "))
			if len(paths) != 1 {
				return b, fmt.Errorf("%w: GRUB loaded %d initrd files; one combined initrd is expected", ErrBadDocument, len(paths))
			}
			initrdPath = paths[0]
		case strings.HasPrefix(c, "kernel_cmdline: "):
			b.cmdlineText = strings.TrimPrefix(c, "kernel_cmdline: ")
		}
	}
	if kernelPath == "" || b.cmdlineText == "" {
		return b, fmt.Errorf("%w: the event log has no GRUB linux command, so the kernel is unknown", ErrBadDocument)
	}
	sum := sha256.Sum256([]byte(b.cmdlineText))
	b.cmdline = sum[:]
	if b.kernel = grubFile(ms, kernelPath); b.kernel == nil {
		return b, fmt.Errorf("%w: GRUB loaded %s but no measurement of it is in the event log", ErrBadDocument, kernelPath)
	}
	if initrdPath != "" {
		if b.initrd = grubFile(ms, initrdPath); b.initrd == nil {
			return b, fmt.Errorf("%w: GRUB loaded %s but no measurement of it is in the event log", ErrBadDocument, initrdPath)
		}
	}
	return b, nil
}

// grubFile returns the digest of the last file GRUB measured under this name.
func grubFile(ms *attest.MachineState, path string) []byte {
	var d []byte
	for _, f := range ms.GetGrub().GetFiles() {
		if strings.TrimRight(string(f.GetUntrustedFilename()), "\x00") == path {
			d = f.GetDigest()
		}
	}
	return d
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

func isZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
