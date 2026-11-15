package tdx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"attestamp/internal/cli"
	"attestamp/internal/setup/build/cpio"
)

// verify builds steps 2 to 4 a second time, from the files on disk, into a fresh folder, and requires the combined
// initrd to come out byte for byte the same. It is the TDX counterpart of Nitro's build --verify: the archive writer is
// fixed by construction, so what it catches is everything else - files that change while being read, an application
// directory that holds something generated, mksquashfs or veritysetup choosing a value of their own, a base initrd that
// was replaced. On a difference it names what differed and fails; the first build's files are already in OutDir, and
// an Auditor could not reproduce them.
func (b Build) verify(ctx context.Context, first artifacts, log io.Writer) error {
	fmt.Fprintf(log, "\n[verify 1/2] building %s again, from the files, into a new folder\n", b.AppDir)
	if b.betweenBuilds != nil {
		if err := b.betweenBuilds(); err != nil {
			return err
		}
	}
	work, err := os.MkdirTemp(b.OutDir, "att-tdx-verify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	second, err := b.make(ctx, work, nil, log)
	if err != nil {
		return err
	}
	fmt.Fprintf(log, "[verify 2/2] comparing the two initrds\n")
	if bytes.Equal(first.initrd, second.initrd) {
		fmt.Fprintln(log, cli.Green("✓ reproducible on this host")+": a second build gave the same initrd, sha384 "+sum384(second.initrd))
		return nil
	}
	d := differences(first, second)
	fmt.Fprintf(log, "%s initrd sha384 build 1 %s / build 2 %s\n", cli.Red("✗ not reproducible."), sum384(first.initrd), sum384(second.initrd))
	for _, line := range d {
		fmt.Fprintln(log, "  "+line)
	}
	return fmt.Errorf("%w: the same application built twice on this host gave two different initrds (%d difference(s) above); the Auditor's rebuild would not match either", ErrNotReproducible, len(d))
}

// differences names what differs between two builds: the base initrd, the dm-verity image, and the archive entries.
func differences(a, b artifacts) []string {
	var out []string
	if !bytes.Equal(a.base, b.base) {
		out = append(out, fmt.Sprintf("base initrd  build 1: sha256 %s  build 2: sha256 %s (the file changed between the builds)", sum256(a.base)[:12], sum256(b.base)[:12]))
	}
	if a.rootHash != b.rootHash {
		out = append(out, fmt.Sprintf("root hash  build 1: %s  build 2: %s", a.rootHash, b.rootHash))
	}
	ea, eb := entryMap(a.entries), entryMap(b.entries)
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
		va, okA := ea[n]
		vb, okB := eb[n]
		switch {
		case !okB:
			out = append(out, "/"+n+"  only in build 1")
		case !okA:
			out = append(out, "/"+n+"  only in build 2")
		case va != vb:
			out = append(out, fmt.Sprintf("/%s  build 1: %s  build 2: %s", n, va, vb))
		}
	}
	if len(out) == 0 {
		out = append(out, "the archive entries are the same, yet the bytes differ (the archive writer itself)")
	}
	return out
}

// entryMap summarises entries as name -> "dir" or "<mode> <sha256 prefix>".
func entryMap(es []cpio.Entry) map[string]string {
	m := map[string]string{}
	for _, e := range es {
		switch {
		case e.Dir:
			m[e.Name] = "dir"
		case e.Executable:
			m[e.Name] = "755 " + sum256(e.Data)[:12]
		default:
			m[e.Name] = "644 " + sum256(e.Data)[:12]
		}
	}
	return m
}
