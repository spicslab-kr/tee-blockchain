// Package snp launches what internal/setup/build/snp built on an AMD SEV-SNP guest on GCP, and reads back what the
// boot measured. It is the role-independent core; internal/setup/launch/{Vendor,auditor}/snp are thin adapters.
//
// Two runs with a reboot between them, as on TDX (docs/plan/cvm-port.md, "Phase 4b design"):
//
//  1. Install (internal/setup/launch/grub, shared with TDX): the initrd into /boot and a GRUB entry for it as the
//     default. For Option 2 also the image and hash tree into /var/lib/attestamp, and the root hash on the command line.
//  2. Check, after the reboot: fresh evidence (the SNP report and the vTPM attestation, bound by SHA3-512) verified by
//     internal/tee/parse/snp, which also replays the vTPM event log. INITRD must be the built initrd's SHA-256; for
//     Option 2, PCR12 must be what the hook extends for the built root hash. Then the values go to measurements.json.
//
// Unlike TDX, the check needs no event-log code of its own: the SEV-SNP parser already returns INITRD and PCR12 from
// verified evidence.
package snp

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
	buildsnp "attestamp/internal/setup/build/snp"
	"attestamp/internal/setup/launch/grub"
	parsesnp "attestamp/internal/tee/parse/snp"
	reqsnp "attestamp/internal/tee/request/snp"
)

// readValue reads and checks a build-value.json.
func readValue(path string) (buildsnp.Value, error) {
	var v buildsnp.Value
	b, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(b, &v); err != nil || (v.Format != buildsnp.FormatInitrd && v.Format != buildsnp.FormatVerity) || len(v.InitrdSHA256) != 64 {
		return v, fmt.Errorf("%s: not a SEV-SNP build value (%v)", path, err)
	}
	if v.Format == buildsnp.FormatVerity && (len(v.RootHash) != 64 || len(v.PCR12) != 64 || v.KernelArgs != buildsnp.RootHashArg+v.RootHash) {
		return v, fmt.Errorf("%s: an %s build value without its root hash, PCR12 and kernel argument", path, buildsnp.FormatVerity)
	}
	return v, nil
}

// Install installs a build: the files next to its build-value.json go to the guest.
type Install struct {
	BuildValue string      // the build's build-value.json; initrd-app.img (and for Option 2 app.img, app.hash) are read from the same folder
	ID         string      // GRUB entry id; "attestamp" when empty
	Root       string      // the guest's root, / unless a test sets another
	Runner     grub.Runner // runs update-grub; nil runs it with os/exec
}

// Run installs. It does not reboot.
func (in Install) Run(ctx context.Context, log io.Writer) (grub.Installed, error) {
	v, err := readValue(in.BuildValue)
	if err != nil {
		return grub.Installed{}, err
	}
	dir := filepath.Dir(in.BuildValue)
	g := grub.Install{Root: in.Root, Initrd: filepath.Join(dir, buildsnp.InitrdFile), ID: in.ID, Runner: in.Runner}
	if v.Format == buildsnp.FormatVerity {
		g.ExtraArgs = v.KernelArgs
		g.Files = []grub.File{
			{Path: buildsnp.ImageDir + "/" + buildsnp.ImageFile, Src: filepath.Join(dir, buildsnp.ImageFile)},
			{Path: buildsnp.ImageDir + "/" + buildsnp.HashTreeFile, Src: filepath.Join(dir, buildsnp.HashTreeFile)},
		}
	}
	fmt.Fprintf(log, "format %s\n", v.Format)
	return g.Run(ctx, log)
}

// Check runs after the guest rebooted into the installed entry.
type Check struct {
	Root       string // the guest's root, / unless a test sets another
	BuildValue string
	OutDir     string // defaults to out
	// Evidence returns the guest's evidence over the nonce. nil asks this guest's ConfigFS TSM and vTPM
	// (internal/tee/request/snp).
	Evidence func(nonce []byte) ([]byte, error)

	nonce  []byte          // tests: a fixed nonce
	parser parsesnp.Parser // tests: a fixed Now for the certificate chains
}

// Measured is what the boot measured.
type Measured struct {
	Format       string
	Values       map[string]string // MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD (and PCR12 for verity): what gets registered
	Measurements string
}

var (
	// ErrInitrdMismatch: INITRD in the verified evidence is not the built initrd. The pass criterion failed.
	ErrInitrdMismatch = errors.New("the measured initrd is not the built one")
	// ErrPCR12Mismatch: the hook did not report the built image as verified.
	ErrPCR12Mismatch = errors.New("PCR12 is not what the built image gives")
	// ErrNotUnpacked: Option 1's archive was measured but not unpacked.
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
	built, err := readValue(c.BuildValue)
	if err != nil {
		return m, err
	}
	m.Format = built.Format
	step := func(n int, what, why string) {
		fmt.Fprintf(log, "\n%s %s…\n            %s\n", cli.Green(fmt.Sprintf("[check %d/3]", n)), what, why)
	}

	step(1, "Asking this guest for evidence over a fresh nonce, and verifying it", "the checks a verifier makes: AMD's chain and the report signature, Google's AK chain and the quotes, REPORT_DATA = SHA3-512 of the vTPM attestation, the nonce, no debug policy, and the event-log replay")
	nonce := c.nonce
	if nonce == nil {
		nonce = make([]byte, 32)
		if _, err := rand.Read(nonce); err != nil {
			return m, err
		}
	}
	evidenceFn := c.Evidence
	if evidenceFn == nil {
		evidenceFn = func(n []byte) ([]byte, error) {
			cl, err := reqsnp.Client()
			if err != nil {
				return nil, err
			}
			return reqsnp.Evidence(cl, reqsnp.GCEAttester{}, n)
		}
	}
	ev, err := evidenceFn(nonce)
	if err != nil {
		return m, err
	}
	p := c.parser
	if err := p.VerifySig(ctx, built.Format, ev, parsesnp.RootPEM()); err != nil {
		return m, err
	}
	if err := p.CheckNonce(built.Format, ev, nonce); err != nil {
		return m, err
	}
	ms, err := p.Parse(built.Format, ev)
	if err != nil {
		return m, err
	}
	m.Values = map[string]string{}
	for _, v := range ms {
		m.Values[v.Name] = hex.EncodeToString(v.Value)
	}

	step(2, "Comparing the measured values with the built ones", "the pass criterion: GRUB measured exactly the initrd that was built, and for Option 2 the hook verified exactly the image that was built")
	if err := compare(built, m.Values); err != nil {
		return m, err
	}
	fmt.Fprintln(log, "    "+cli.Green("✓ the same"))

	step(3, "Checking that the application was really unpacked or mounted", "a measured initrd is not proof: in 2026-11 an archive the kernel dropped was still measured")
	if err := checkMarker(c.Root, built); err != nil {
		return m, err
	}

	if err := os.MkdirAll(c.OutDir, 0o755); err != nil {
		return m, err
	}
	m.Measurements = filepath.Join(c.OutDir, "measurements.json")
	b, err := json.MarshalIndent(map[string]map[string]string{"Measurements": m.Values}, "", "  ")
	if err != nil {
		return m, err
	}
	if err := os.WriteFile(m.Measurements, append(b, '\n'), 0o644); err != nil {
		return m, err
	}
	if err := os.WriteFile(filepath.Join(c.OutDir, "selfcheck.evidence"), ev, 0o644); err != nil {
		return m, err
	}
	if err := os.WriteFile(filepath.Join(c.OutDir, "selfcheck.evidence.nonce"), []byte(hex.EncodeToString(nonce)+"\n"), 0o644); err != nil {
		return m, err
	}
	fmt.Fprintf(log, "%s: these values identify the boot (%s), and go on the chain\n", cli.Green("✓ measured"), m.Format)
	for _, k := range parsesnp.MeasurementNames(m.Format) {
		fmt.Fprintf(log, "    %-11s %s\n", k, m.Values[k])
	}
	return m, nil
}

// compare is the pass criterion: INITRD equals the build's, and for Option 2 PCR12 equals the predicted value.
func compare(built buildsnp.Value, values map[string]string) error {
	if values["INITRD"] != built.InitrdSHA256 {
		return fmt.Errorf("%w: INITRD %s, the build made %s", ErrInitrdMismatch, values["INITRD"], built.InitrdSHA256)
	}
	if built.Format == buildsnp.FormatVerity && values["PCR12"] != built.PCR12 {
		return fmt.Errorf("%w: PCR12 %s, the built image (root hash %s) gives %s when it verifies", ErrPCR12Mismatch, values["PCR12"], built.RootHash, built.PCR12)
	}
	return nil
}

// checkMarker: Option 1 needs the boot marker; Option 2 needs the SEV-SNP hook's status "ok".
func checkMarker(root string, built buildsnp.Value) error {
	if built.Format == buildsnp.FormatInitrd {
		if _, err := os.Stat(filepath.Join(root, "dev", "attestamp-app-marker")); err != nil {
			return fmt.Errorf("%w: /dev/attestamp-app-marker is missing (%v)", ErrNotUnpacked, err)
		}
		return nil
	}
	status, err := os.ReadFile(filepath.Join(root, "run", "attestamp-status"))
	if err != nil {
		return fmt.Errorf("%w: /run/attestamp-status is missing; does the base initrd carry the verity hook? (%v)", ErrVerity, err)
	}
	if got := strings.TrimSpace(string(status)); got != "ok" {
		return fmt.Errorf("%w: /run/attestamp-status reads %q", ErrVerity, got)
	}
	return nil
}
