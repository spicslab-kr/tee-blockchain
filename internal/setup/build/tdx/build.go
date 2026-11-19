// Package tdx builds the initrd that carries an application into an Intel TDX guest, and records the value the guest's
// boot will measure from it. It is the role-independent core; internal/setup/build/{Vendor,auditor}/tdx are thin
// adapters, the same two-layer shape as internal/tee/parse and internal/tee/request.
//
// It follows the five steps of the Nitro build (internal/setup/build/aws/build.go), so the platforms read alike;
// STRUCTURE.md, "Build and launch on the three platforms", has the side-by-side table:
//
//  1. tools: mksquashfs and veritysetup for Option 2, with their versions recorded
//  2. the software: the application tree, read as files
//  3. what we add: the boot marker, and for Option 2 the squashfs + dm-verity image of the tree
//  4. reproducible bytes: the cpio archive with fixed metadata, placed before the Vendor's base initrd (cpio.AppFirst)
//  5. the measured thing: the combined initrd and its SHA-384, the digest GRUB extends into RTMR2
//
// The four registered values (MRTD, RTMR0-2) cannot be computed here: they depend on the firmware, the boot loader,
// the kernel and the GRUB commands of the machine. internal/setup/launch/tdx boots once and reads them back.
package tdx

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
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
	"attestamp/internal/setup/build/verity"
)

// Format is the format string a TDX guest's quote is labelled with. Option 1 and Option 2 both register under it:
// both move RTMR2 only (docs/build/tdx.md), so the measurement names do not change.
const Format = "intel-tdx"

// The two ways to carry the application (docs/build/tdx.md, "Option 1 and Option 2").
const (
	Option1 = 1 // the application tree as a cpio archive in the initrd, under /opt/app
	Option2 = 2 // a squashfs image of the tree with a dm-verity hash tree, carried in the initrd under /opt/app
)

// File names under OutDir.
const (
	InitrdFile = "initrd-app.img"   // the combined initrd: the application archive, then the Vendor's base initrd
	ValueFile  = "build-value.json" // Value, read by launch to check the boot
)

// MarkerScript is the boot marker: initramfs-tools' /init sources every file in conf/conf.d, so after the boot
// /dev/attestamp-app-marker exists exactly when the kernel unpacked the application archive. A measurement that changes
// with the archive does not prove the archive was unpacked (docs/build/tdx.md, "Before or after the base initrd"); this
// file does. It is byte for byte the marker of cpio/testdata/mkappinitrd, with which the fixtures were recorded
// (build_test.go checks that).
const MarkerScript = `# attestamp app-fixture marker: proves that the application archive was unpacked into the initramfs.
if [ -d /opt/app ]; then
	{
		for f in /opt/app/*; do [ -f "$f" ] && echo "file ${f##*/} $(wc -c < "$f")"; done
		[ -f /opt/app/app ] && cat /opt/app/app
		[ -f /opt/app/roothash ] && cat /opt/app/roothash
	} > /dev/attestamp-app-marker 2>/dev/null || true
fi
`

// MarkerPath is where the marker writes, on the booted guest.
const MarkerPath = "/dev/attestamp-app-marker"

const markerEntry = "conf/conf.d/zz-attestamp-app-marker"

// Build is the input to one build.
type Build struct {
	AppDir     string // the application: a directory tree of regular files, put under /opt/app
	BaseInitrd string // the Vendor's published base initrd (STRUCTURE.md, "Build and launch on the three platforms"). For Option 2 it must carry the dm-verity hook
	Option     int    // Option1 or Option2
	OutDir     string // defaults to out
	Verify     bool   // build a second time from the files and fail unless the bytes are the same (verify.go)

	// Runner runs mksquashfs and veritysetup. nil runs them with os/exec.
	Runner Runner

	goos          string       // runtime.GOOS unless a test sets it
	betweenBuilds func() error // tests: runs between the two builds of Verify
}

// Value is what the build knows of the measurement, written to OutDir/build-value.json.
type Value struct {
	Format           string            `json:"format"`
	Option           int               `json:"option"`
	InitrdSHA384     string            `json:"initrd_sha384"` // what GRUB measures into RTMR2; launch compares the boot's event with it
	InitrdSHA256     string            `json:"initrd_sha256"`
	InitrdSize       int               `json:"initrd_size"`
	BaseInitrdSHA256 string            `json:"base_initrd_sha256"`
	ArchiveSHA256    string            `json:"archive_sha256"`
	RootHash         string            `json:"root_hash,omitempty"` // Option 2: the dm-verity root hash, carried in the archive
	ImageSHA256      string            `json:"image_sha256,omitempty"`
	HashTreeSHA256   string            `json:"hash_tree_sha256,omitempty"`
	Tools            map[string]string `json:"tools,omitempty"` // first line of each tool's version output (Option 2)
	Note             string            `json:"note"`
}

// Result is what one build produced.
type Result struct {
	Initrd string // OutDir/initrd-app.img
	Record string // OutDir/build-value.json
	Value  Value
}

// ErrNotLinux: the build runs on a Linux machine. On Windows the files have no execute bits, so the archive would differ.
var ErrNotLinux = errors.New("the TDX build runs on Linux (file modes are part of the archive)")

// ErrNotReproducible is returned by Verify when the second build differs from the first.
var ErrNotReproducible = errors.New("not reproducible")

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
	if err := build.CheckToolchainFor("intel-tdx"); err != nil {
		return res, err
	}
	if b.AppDir == "" || b.BaseInitrd == "" {
		return res, errors.New("the application directory and the base initrd are both required")
	}
	if b.Option != Option1 && b.Option != Option2 {
		return res, fmt.Errorf("option %d: 1 (application archive in the initrd) or 2 (squashfs + dm-verity image in the initrd)", b.Option)
	}
	if b.OutDir == "" {
		b.OutDir = "out"
	}
	if b.Runner == nil {
		b.Runner = ExecRunner{}
	}
	if err := os.MkdirAll(b.OutDir, 0o755); err != nil {
		return res, err
	}
	n := 0
	step := func(what, why string) {
		n++
		fmt.Fprintf(log, "\n%s %s…\n           %s\n", cli.Green(fmt.Sprintf("[build %d/5]", n)), what, why)
	}

	if b.Option == Option2 {
		step("Checking the tools", "Option 2 makes the image with mksquashfs and veritysetup; their versions are part of what makes the bytes, so they are recorded")
	} else {
		step("Checking the tools", "Option 1 needs none: this binary writes the application archive itself")
	}
	tools, err := b.tools(ctx)
	if err != nil {
		return res, err
	}
	for _, k := range sortedKeys(tools) {
		fmt.Fprintf(log, "    %s: %s\n", k, tools[k])
	}

	work, err := os.MkdirTemp(b.OutDir, "att-tdx-*")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(work)
	first, err := b.make(ctx, work, step, log)
	if err != nil {
		return res, err
	}

	step("Writing the initrd and the value the boot will measure → "+filepath.Join(b.OutDir, InitrdFile), "GRUB loads this file and extends its SHA-384 into RTMR2; launch boots it once and checks that exactly this digest was measured")
	res.Initrd = filepath.Join(b.OutDir, InitrdFile)
	if err := os.WriteFile(res.Initrd, first.initrd, 0o644); err != nil {
		return res, err
	}
	res.Value = first.value(b.Option, tools)
	res.Record = filepath.Join(b.OutDir, ValueFile)
	if err := writeJSON(res.Record, res.Value); err != nil {
		return res, err
	}
	fmt.Fprintln(log, cli.Green("✓ built")+":", "the initrd GRUB will measure")
	fmt.Fprintf(log, "    initrd sha384 %s\n", res.Value.InitrdSHA384)
	if res.Value.RootHash != "" {
		fmt.Fprintf(log, "    root hash     %s\n", res.Value.RootHash)
	}
	fmt.Fprintln(log, "    MRTD and RTMR0-2, the values that get registered, are read by launch after one boot")

	if b.Verify {
		if err := b.verify(ctx, first, log); err != nil {
			return res, err
		}
	}
	return res, nil
}

// artifacts is one build's output, held in memory so two builds can be compared.
type artifacts struct {
	entries         []cpio.Entry
	archive, base   []byte
	initrd          []byte
	image, hashTree []byte // Option 2
	rootHash        string // Option 2
}

func (a artifacts) value(option int, tools map[string]string) Value {
	v := Value{
		Format:           Format,
		Option:           option,
		InitrdSHA384:     sum384(a.initrd),
		InitrdSHA256:     sum256(a.initrd),
		InitrdSize:       len(a.initrd),
		BaseInitrdSHA256: sum256(a.base),
		ArchiveSHA256:    sum256(a.archive),
		Tools:            tools,
		Note:             "MRTD, RTMR0, RTMR1 and RTMR2 depend on the machine's firmware, boot loader, kernel and GRUB commands; launch boots this initrd once and reads them back",
	}
	if option == Option2 {
		v.RootHash, v.ImageSHA256, v.HashTreeSHA256 = a.rootHash, sum256(a.image), sum256(a.hashTree)
	}
	return v
}

// make runs steps 2 to 4 into work. step prints a step header; nil prints nothing (the second build of Verify).
func (b Build) make(ctx context.Context, work string, step func(what, why string), log io.Writer) (artifacts, error) {
	var a artifacts
	say := func(what, why string) {
		if step != nil {
			step(what, why)
		}
	}
	say("Reading the application from "+b.AppDir, "the software as files: what is on disk is what goes in, so a link or a device file is refused rather than followed or skipped")
	tree, err := cpio.FromDir(b.AppDir)
	if err != nil {
		return a, err
	}
	if step != nil {
		fmt.Fprintf(log, "    %d entries\n", len(tree))
	}

	a.entries = []cpio.Entry{
		{Name: "conf", Dir: true},
		{Name: "conf/conf.d", Dir: true},
		{Name: markerEntry, Data: []byte(MarkerScript)},
		{Name: "opt", Dir: true},
		{Name: "opt/app", Dir: true},
	}
	if b.Option == Option1 {
		say("Adding the boot marker", "a small script the initramfs runs; after the boot its output shows the application archive was really unpacked, which a measurement alone cannot")
		for _, e := range tree {
			if e.Name == "." {
				continue // opt/app itself, already there
			}
			e.Name = "opt/app/" + e.Name
			a.entries = append(a.entries, e)
		}
	} else {
		say("Adding the boot marker and making the squashfs + dm-verity image", "the image is read-only and every block is checked against the root hash when it is read; the base initrd's hook opens it and mounts it on /opt/app-verity")
		im, err := verity.Make(ctx, b.Runner, work, tree)
		if err != nil {
			return a, err
		}
		a.image, a.hashTree, a.rootHash = im.Data, im.HashTree, im.RootHash
		a.entries = append(a.entries,
			cpio.Entry{Name: "opt/app/app.img", Data: a.image},
			cpio.Entry{Name: "opt/app/app.hash", Data: a.hashTree},
			cpio.Entry{Name: "opt/app/roothash", Data: []byte(a.rootHash + "\n")},
		)
		if step != nil {
			fmt.Fprintf(log, "    image %d bytes, root hash %s\n", len(a.image), a.rootHash)
		}
	}

	say("Writing the archive and putting it before the base initrd", "every header field that is not content is fixed (name order, time 0, owner 0), so the same files give the same bytes on every machine; the archive goes first (cpio.AppFirst), as the TDX fixtures were recorded")
	if a.archive, err = cpio.Bytes(a.entries); err != nil {
		return a, err
	}
	if a.base, err = os.ReadFile(b.BaseInitrd); err != nil {
		return a, err
	}
	a.initrd = cpio.Join(a.base, a.archive, cpio.AppFirst)
	if step != nil {
		fmt.Fprintf(log, "    archive %d bytes, base %d bytes (sha256 %s), initrd %d bytes\n", len(a.archive), len(a.base), sum256(a.base)[:16], len(a.initrd))
	}
	return a, nil
}

func (b Build) tools(ctx context.Context) (map[string]string, error) {
	if b.Option != Option2 {
		return nil, nil
	}
	return verity.Tools(ctx, b.Runner)
}

// Runner runs mksquashfs and veritysetup (internal/setup/build/verity).
type Runner = verity.Runner

// ExecRunner runs them with os/exec.
type ExecRunner = verity.ExecRunner

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func sum256(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func sum384(b []byte) string { s := sha512.Sum384(b); return hex.EncodeToString(s[:]) }

func sortedKeys(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
