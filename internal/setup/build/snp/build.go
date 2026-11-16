// Package snp builds what carries an application into an AMD SEV-SNP guest on GCP, and records the values the guest's
// boot will measure from it. It is the role-independent core; internal/setup/build/{Vendor,auditor}/snp are thin
// adapters. It has the five steps of the Nitro and TDX builds; docs/plan/cvm-port.md, "Phase 4b design", has what
// differs from TDX:
//
//   - Option 1 (amd-sev-snp-initrd): the application archive goes after the base initrd, which is zero-padded to a
//     multiple of 4 bytes first (cpio.AppLast). Without the padding the kernel drops the archive and still boots.
//   - Option 2 (amd-sev-snp-verity): the application is a squashfs + dm-verity image that stays on disk
//     (/var/lib/attestamp). The initrd is the Vendor's base initrd, built with the verity hook, unchanged; the root hash
//     goes on the kernel command line, and the hook extends vTPM PCR12 with the result. So the build can predict PCR12.
//
// What the build cannot predict: MEASUREMENT (the firmware), PCR4 (shim and GRUB), KERNEL and CMDLINE (the machine's
// kernel and command line). internal/setup/launch/snp boots once and reads them from verified evidence.
package snp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"attestamp/internal/cli"
	"attestamp/internal/setup/build"
	"attestamp/internal/setup/build/cpio"
	"attestamp/internal/setup/build/tdx"
	"attestamp/internal/setup/build/verity"
)

// The two format strings: each option registers a different list of values (docs/build/sev-snp.md, "Decision").
const (
	FormatInitrd = "amd-sev-snp-initrd"
	FormatVerity = "amd-sev-snp-verity"
)

const (
	Option1 = 1 // the application tree as a cpio archive appended to the initrd
	Option2 = 2 // a squashfs + dm-verity image on disk, checked at boot, the result extended into PCR12
)

// FormatOf returns the format an option registers under.
func FormatOf(option int) string {
	if option == Option2 {
		return FormatVerity
	}
	return FormatInitrd
}

// File names under OutDir.
const (
	InitrdFile   = "initrd-app.img"
	ImageFile    = "app.img"
	HashTreeFile = "app.hash"
	ValueFile    = "build-value.json"
)

// ImageDir is where the Option 2 image and its hash tree go on the guest: the verity hook reads them from there
// (internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh, ${rootmnt}/var/lib/attestamp).
const ImageDir = "/var/lib/attestamp"

// RootHashArg is the kernel command line argument the verity hook reads the root hash from; internal/tee/parse/snp
// refuses an amd-sev-snp-verity boot without it.
const RootHashArg = "attestamp.roothash="

// MarkerScript is the same boot marker as on TDX (cpio/testdata/mkappinitrd's): Option 1 carries it in the archive.
const MarkerScript = tdx.MarkerScript

// Build is the input to one build.
type Build struct {
	AppDir     string // the application tree
	BaseInitrd string // the Vendor's published base initrd. For Option 2 it must have been built with the verity hook
	Option     int    // Option1 or Option2
	OutDir     string // defaults to out
	Verify     bool   // build a second time from the files and fail unless everything is the same
	Runner     verity.Runner

	goos          string
	betweenBuilds func() error
}

// Value is what the build knows of the measurement, written to OutDir/build-value.json.
type Value struct {
	Format           string            `json:"format"`
	Option           int               `json:"option"`
	InitrdSHA256     string            `json:"initrd_sha256"` // INITRD: what GRUB measures; launch compares the verified evidence with it
	InitrdSize       int               `json:"initrd_size"`
	BaseInitrdSHA256 string            `json:"base_initrd_sha256"`
	ArchiveSHA256    string            `json:"archive_sha256,omitempty"` // Option 1
	RootHash         string            `json:"root_hash,omitempty"`      // Option 2
	ImageSHA256      string            `json:"image_sha256,omitempty"`
	HashTreeSHA256   string            `json:"hash_tree_sha256,omitempty"`
	PCR12            string            `json:"pcr12,omitempty"`       // Option 2: what the hook extends PCR12 to when the image verifies
	KernelArgs       string            `json:"kernel_args,omitempty"` // Option 2: to append to the kernel command line
	Tools            map[string]string `json:"tools,omitempty"`
	Note             string            `json:"note"`
}

// Result is what one build produced.
type Result struct {
	Initrd   string // OutDir/initrd-app.img
	Image    string // Option 2: OutDir/app.img
	HashTree string // Option 2: OutDir/app.hash
	Record   string // OutDir/build-value.json
	Value    Value
}

var (
	ErrNotLinux        = errors.New("the SEV-SNP build runs on Linux (file modes are part of the archive and the image)")
	ErrNotReproducible = errors.New("not reproducible")
)

// Run goes through the five steps and, with Verify, builds once more and compares.
func (b Build) Run(ctx context.Context, log io.Writer) (Result, error) {
	var res Result
	goos := b.goos
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "linux" {
		return res, ErrNotLinux
	}
	if err := build.CheckToolchainFor(FormatOf(b.Option)); err != nil {
		return res, err
	}
	if b.AppDir == "" || b.BaseInitrd == "" {
		return res, errors.New("the application directory and the base initrd are both required")
	}
	if b.Option != Option1 && b.Option != Option2 {
		return res, fmt.Errorf("option %d: 1 (%s, the archive appended to the initrd) or 2 (%s, a dm-verity image on disk)", b.Option, FormatInitrd, FormatVerity)
	}
	if b.OutDir == "" {
		b.OutDir = "out"
	}
	if b.Runner == nil {
		b.Runner = verity.ExecRunner{}
	}
	if err := os.MkdirAll(b.OutDir, 0o755); err != nil {
		return res, err
	}
	n := 0
	step := func(what, why string) {
		n++
		fmt.Fprintf(log, "\n%s %s…\n           %s\n", cli.Green(fmt.Sprintf("[build %d/5]", n)), what, why)
	}
	fmt.Fprintf(log, "format %s (option %d)\n", FormatOf(b.Option), b.Option)

	step("Checking the tools", "Option 2 makes the image with mksquashfs and veritysetup; their versions are part of what makes the bytes")
	var tools map[string]string
	if b.Option == Option2 {
		var err error
		if tools, err = verity.Tools(ctx, b.Runner); err != nil {
			return res, err
		}
		for _, k := range []string{"mksquashfs", "veritysetup"} {
			fmt.Fprintf(log, "    %s: %s\n", k, tools[k])
		}
	}

	work, err := os.MkdirTemp(b.OutDir, "att-snp-*")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(work)
	first, err := b.make(ctx, work, step, log)
	if err != nil {
		return res, err
	}

	step("Writing what the guest needs and the values the boot will measure", "GRUB measures the initrd's SHA-256 (INITRD); for Option 2 the hook extends PCR12 from the root hash, so both are known now")
	res.Initrd = filepath.Join(b.OutDir, InitrdFile)
	if err := os.WriteFile(res.Initrd, first.initrd, 0o644); err != nil {
		return res, err
	}
	if b.Option == Option2 {
		res.Image, res.HashTree = filepath.Join(b.OutDir, ImageFile), filepath.Join(b.OutDir, HashTreeFile)
		if err := os.WriteFile(res.Image, first.image, 0o644); err != nil {
			return res, err
		}
		if err := os.WriteFile(res.HashTree, first.hashTree, 0o644); err != nil {
			return res, err
		}
	}
	res.Value = first.value(b.Option, tools)
	res.Record = filepath.Join(b.OutDir, ValueFile)
	if err := writeJSON(res.Record, res.Value); err != nil {
		return res, err
	}
	fmt.Fprintln(log, cli.Green("✓ built")+":")
	fmt.Fprintf(log, "    INITRD (sha256) %s\n", res.Value.InitrdSHA256)
	if b.Option == Option2 {
		fmt.Fprintf(log, "    root hash       %s\n    PCR12           %s\n    kernel args     %s\n", res.Value.RootHash, res.Value.PCR12, res.Value.KernelArgs)
	}
	fmt.Fprintln(log, "    MEASUREMENT, PCR4, KERNEL and CMDLINE are read by launch after one boot")

	if b.Verify {
		if err := b.verify(ctx, first, log); err != nil {
			return res, err
		}
	}
	return res, nil
}

type artifacts struct {
	entries         []cpio.Entry // Option 1: the archive's entries
	archive, base   []byte
	initrd          []byte
	image, hashTree []byte // Option 2
	rootHash        string
}

func (a artifacts) value(option int, tools map[string]string) Value {
	v := Value{
		Format:           FormatOf(option),
		Option:           option,
		InitrdSHA256:     sum256(a.initrd),
		InitrdSize:       len(a.initrd),
		BaseInitrdSHA256: sum256(a.base),
		Tools:            tools,
		Note:             "MEASUREMENT, PCR4, KERNEL and CMDLINE depend on the machine's firmware, boot loader, kernel and command line; launch boots once and reads them from the verified evidence",
	}
	if option == Option1 {
		v.ArchiveSHA256 = sum256(a.archive)
	} else {
		v.RootHash, v.ImageSHA256, v.HashTreeSHA256 = a.rootHash, sum256(a.image), sum256(a.hashTree)
		v.PCR12 = hex.EncodeToString(PCR12(a.rootHash))
		v.KernelArgs = RootHashArg + a.rootHash
	}
	return v
}

// PCR12 is what the verity hook leaves in vTPM PCR12 (SHA-256 bank) when the image verifies against rootHash: one
// extend of a zero PCR with SHA-256("attestamp-verity-ok:" || root hash), as initramfs-local-bottom.sh does and as the
// Option 2 recordings showed (internal/tee/parse/snp/app_fixture_test.go).
func PCR12(rootHash string) []byte {
	d := sha256.Sum256([]byte("attestamp-verity-ok:" + rootHash))
	p := sha256.Sum256(append(make([]byte, 32), d[:]...))
	return p[:]
}

// make runs steps 2 to 4 into work. step nil prints nothing (the second build of Verify).
func (b Build) make(ctx context.Context, work string, step func(what, why string), log io.Writer) (artifacts, error) {
	var a artifacts
	say := func(what, why string) {
		if step != nil {
			step(what, why)
		}
	}
	say("Reading the application from "+b.AppDir, "the software as files: a link or a device file is refused rather than followed or skipped")
	tree, err := cpio.FromDir(b.AppDir)
	if err != nil {
		return a, err
	}
	if a.base, err = os.ReadFile(b.BaseInitrd); err != nil {
		return a, err
	}
	if b.Option == Option1 {
		say("Adding the boot marker", "after the boot its output shows the archive was really unpacked; in 2026-11 an archive the kernel dropped was still measured")
		a.entries = []cpio.Entry{
			{Name: "conf", Dir: true},
			{Name: "conf/conf.d", Dir: true},
			{Name: "conf/conf.d/zz-attestamp-app-marker", Data: []byte(MarkerScript)},
			{Name: "opt", Dir: true},
			{Name: "opt/app", Dir: true},
		}
		for _, e := range tree {
			if e.Name != "." {
				e.Name = "opt/app/" + e.Name
				a.entries = append(a.entries, e)
			}
		}
		say("Writing the archive and appending it to the base initrd, padded to 4 bytes", "fixed metadata gives the same bytes everywhere; the kernel only finds an archive after a compressed one at a 4-byte boundary (cpio.AppLast)")
		if a.archive, err = cpio.Bytes(a.entries); err != nil {
			return a, err
		}
		a.initrd = cpio.Join(a.base, a.archive, cpio.AppLast)
	} else {
		say("Making the squashfs + dm-verity image", "it stays on disk; at boot the base initrd's hook checks every block against the root hash, mounts it read-only and extends PCR12 with the result")
		im, err := verity.Make(ctx, b.Runner, work, tree)
		if err != nil {
			return a, err
		}
		a.image, a.hashTree, a.rootHash = im.Data, im.HashTree, im.RootHash
		say("Taking the base initrd as it is", "it carries the verity hook; the image is not in it, so the root hash goes on the kernel command line instead, where CMDLINE measures it")
		a.initrd = a.base
	}
	if step != nil {
		fmt.Fprintf(log, "    initrd %d bytes (base %d bytes)\n", len(a.initrd), len(a.base))
	}
	return a, nil
}

// verify builds steps 2 to 4 a second time from the files and requires the initrd, and for Option 2 the image, its
// hash tree and the root hash, to be byte for byte the same.
func (b Build) verify(ctx context.Context, first artifacts, log io.Writer) error {
	fmt.Fprintf(log, "\n[verify 1/2] building %s again, from the files, into a new folder\n", b.AppDir)
	if b.betweenBuilds != nil {
		if err := b.betweenBuilds(); err != nil {
			return err
		}
	}
	work, err := os.MkdirTemp(b.OutDir, "att-snp-verify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	second, err := b.make(ctx, work, nil, log)
	if err != nil {
		return err
	}
	fmt.Fprintf(log, "[verify 2/2] comparing the two builds\n")
	if bytes.Equal(first.initrd, second.initrd) && bytes.Equal(first.image, second.image) && bytes.Equal(first.hashTree, second.hashTree) {
		fmt.Fprintln(log, cli.Green("✓ reproducible on this host")+": a second build gave the same bytes")
		return nil
	}
	d := differences(first, second)
	fmt.Fprintln(log, cli.Red("✗ not reproducible."))
	for _, line := range d {
		fmt.Fprintln(log, "  "+line)
	}
	return fmt.Errorf("%w: the same application built twice on this host differs (%d difference(s) above); the Auditor's rebuild would not match either", ErrNotReproducible, len(d))
}

func differences(a, b artifacts) []string {
	var out []string
	if !bytes.Equal(a.base, b.base) {
		out = append(out, "base initrd changed between the builds")
	}
	if a.rootHash != b.rootHash {
		out = append(out, fmt.Sprintf("root hash  build 1: %s  build 2: %s", a.rootHash, b.rootHash))
	}
	ea, eb := map[string]string{}, map[string]string{}
	for _, e := range a.entries {
		ea[e.Name] = fmt.Sprintf("%v %v %s", e.Dir, e.Executable, sum256(e.Data)[:12])
	}
	for _, e := range b.entries {
		eb[e.Name] = fmt.Sprintf("%v %v %s", e.Dir, e.Executable, sum256(e.Data)[:12])
	}
	var names []string
	for n := range ea {
		names = append(names, n)
	}
	for n := range eb {
		if _, ok := ea[n]; !ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		if ea[n] != eb[n] {
			out = append(out, fmt.Sprintf("/%s  build 1: %q  build 2: %q", n, ea[n], eb[n]))
		}
	}
	if len(out) == 0 {
		out = append(out, "the inputs read the same, yet the bytes differ")
	}
	return out
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func sum256(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
