package tdx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"attestamp/internal/cli"
	buildtdx "attestamp/internal/setup/build/tdx"
	parsetdx "attestamp/internal/tee/parse/tdx"
	reqtdx "attestamp/internal/tee/request/tdx"
)

// Where the guest's kernel exposes the CC event log (the ACPI CCEL table's data).
const ccelPath = "sys/firmware/acpi/tables/data/CCEL"

// Check runs after the guest rebooted into the installed entry.
type Check struct {
	Root       string // the guest's root, / unless a test sets another
	BuildValue string // the build's build-value.json
	OutDir     string // defaults to out
	// Quote returns a quote over the nonce. nil asks this guest's ConfigFS TSM (internal/tee/request/tdx).
	Quote func(nonce []byte) ([]byte, error)

	nonce []byte // tests: a fixed nonce instead of a random one
}

// Measured is what the boot measured.
type Measured struct {
	Values       map[string]string // MRTD, RTMR0, RTMR1, RTMR2: what gets registered
	Measurements string            // OutDir/measurements.json, in the shape register and audit read
	InitrdPath   string
	InitrdSHA384 string
	Cmdline      string
}

var (
	// ErrInitrdMismatch: GRUB measured an initrd other than the one built. The pass criterion of Phase 3.5 failed.
	ErrInitrdMismatch = errors.New("the measured initrd is not the built one")
	// ErrNotUnpacked: the boot marker is missing, so the kernel did not unpack the application archive even though
	// the initrd was measured (docs/build/tdx.md, "Before or after the base initrd").
	ErrNotUnpacked = errors.New("the application archive was not unpacked")
	// ErrVerity: Option 2's image was not verified and mounted at boot.
	ErrVerity = errors.New("the dm-verity image was not opened")
)

// Run checks the boot and writes the measurements.
func (c Check) Run(ctx context.Context, log io.Writer) (Measured, error) {
	var m Measured
	if c.Root == "" {
		c.Root = "/"
	}
	if c.OutDir == "" {
		c.OutDir = "out"
	}
	var built buildtdx.Value
	b, err := os.ReadFile(c.BuildValue)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &built); err != nil || built.Format != buildtdx.Format || len(built.InitrdSHA384) != 96 {
		return m, fmt.Errorf("%s: not a TDX build value (%v)", c.BuildValue, err)
	}
	step := func(n int, what, why string) {
		fmt.Fprintf(log, "\n%s %s…\n            %s\n", cli.Green(fmt.Sprintf("[check %d/4]", n)), what, why)
	}

	step(1, "Asking this guest for a quote over a fresh nonce, and verifying it", "the same checks a verifier makes: Intel's signature chain, the nonce, and no debug mode")
	nonce := c.nonce
	if nonce == nil {
		nonce = make([]byte, 32)
		if _, err := rand.Read(nonce); err != nil {
			return m, err
		}
	}
	quoteFn := c.Quote
	if quoteFn == nil {
		quoteFn = func(n []byte) ([]byte, error) {
			cl, err := reqtdx.Client()
			if err != nil {
				return nil, err
			}
			return reqtdx.Quote(cl, n)
		}
	}
	quote, err := quoteFn(nonce)
	if err != nil {
		return m, err
	}
	var p parsetdx.Parser
	if err := p.VerifySig(ctx, parsetdx.Format, quote, parsetdx.RootPEM()); err != nil {
		return m, err
	}
	if err := p.CheckNonce(parsetdx.Format, quote, nonce); err != nil {
		return m, err
	}
	ms, err := p.Parse(parsetdx.Format, quote) // refuses a debug TD
	if err != nil {
		return m, err
	}
	m.Values = map[string]string{}
	for _, v := range ms {
		m.Values[v.Name] = hex.EncodeToString(v.Value)
	}

	step(2, "Replaying the CC event log against the quote's RTMR0-2", "the log is not signed; only a replay that ends at the quoted values ties it to the quote, and only then is what it says about GRUB believed")
	raw, err := os.ReadFile(filepath.Join(c.Root, ccelPath))
	if err != nil {
		return m, fmt.Errorf("%w: %v (the kernel exposes the CC event log there on a TDX guest)", ErrEventLog, err)
	}
	g, err := grubFromLog(raw, [3][]byte{ms.Get("RTMR0"), ms.Get("RTMR1"), ms.Get("RTMR2")})
	if err != nil {
		return m, err
	}
	m.InitrdPath, m.InitrdSHA384, m.Cmdline = g.initrdPath, hex.EncodeToString(g.initrdDigest), g.cmdline
	fmt.Fprintf(log, "    GRUB loaded %s, sha384 %s\n    command line %s\n", m.InitrdPath, m.InitrdSHA384, m.Cmdline)

	step(3, "Comparing the measured initrd with the built one", "the pass criterion: the boot measured exactly the initrd that was built")
	if err := compareInitrd(built, g); err != nil {
		return m, err
	}
	fmt.Fprintln(log, "    "+cli.Green("✓ the same"))

	step(4, "Checking that the application was really unpacked", "a measured initrd is not proof: in 2026-11 an archive the kernel dropped was still measured. The boot marker is")
	if err := checkMarker(c.Root, built); err != nil {
		return m, err
	}

	if err := os.MkdirAll(c.OutDir, 0o755); err != nil {
		return m, err
	}
	m.Measurements = filepath.Join(c.OutDir, "measurements.json")
	if err := writeMeasurements(m.Measurements, m.Values); err != nil {
		return m, err
	}
	if err := os.WriteFile(filepath.Join(c.OutDir, "selfcheck.quote"), quote, 0o644); err != nil {
		return m, err
	}
	if err := os.WriteFile(filepath.Join(c.OutDir, "selfcheck.quote.nonce"), []byte(hex.EncodeToString(nonce)+"\n"), 0o644); err != nil {
		return m, err
	}
	fmt.Fprintln(log, cli.Green("✓ measured")+": these four values identify the boot, and go on the chain")
	for _, k := range []string{"MRTD", "RTMR0", "RTMR1", "RTMR2"} {
		fmt.Fprintf(log, "    %-5s %s\n", k, m.Values[k])
	}
	return m, nil
}

// compareInitrd is the pass criterion of Phase 3.5: the initrd GRUB measured is the one built.
func compareInitrd(built buildtdx.Value, g grubBoot) error {
	if got := hex.EncodeToString(g.initrdDigest); got != built.InitrdSHA384 {
		return fmt.Errorf("%w: GRUB measured %s with sha384 %s, the build made %s", ErrInitrdMismatch, g.initrdPath, got, built.InitrdSHA384)
	}
	return nil
}

// checkMarker requires the boot marker, and for Option 2 the dm-verity hook's status line naming the built root hash.
func checkMarker(root string, built buildtdx.Value) error {
	marker, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(buildtdx.MarkerPath, "/")))
	if err != nil {
		return fmt.Errorf("%w: %s is missing (%v)", ErrNotUnpacked, buildtdx.MarkerPath, err)
	}
	if built.Option != buildtdx.Option2 {
		return nil
	}
	if !strings.Contains(string(marker), built.RootHash) {
		return fmt.Errorf("%w: the marker does not name the built root hash %s", ErrNotUnpacked, built.RootHash)
	}
	status, err := os.ReadFile(filepath.Join(root, "run", "attestamp-status"))
	if err != nil {
		return fmt.Errorf("%w: /run/attestamp-status is missing; does the base initrd carry the dm-verity hook? (%v)", ErrVerity, err)
	}
	if got := strings.TrimSpace(string(status)); got != "ok "+built.RootHash {
		return fmt.Errorf("%w: /run/attestamp-status reads %q, want \"ok %s\"", ErrVerity, got, built.RootHash)
	}
	return nil
}

// writeMeasurements writes {"Measurements": {...}}, the shape nitro-cli writes and readMeasurements in cmd/* reads.
func writeMeasurements(path string, values map[string]string) error {
	b, err := json.MarshalIndent(map[string]map[string]string{"Measurements": values}, "", "  ") // keys come out sorted
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
