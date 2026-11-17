package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"attestamp/internal/cli"
)

// manifest is a per-path summary of the extracted rootfs after normalization's cleanups and just before repacking: path -> "mode f sha256" for files,
// "mode l → target" for links and "mode d" for directories. It covers what goes into the measurement (content and mode; repacking sets times and owners
// to 0). When two builds differ, it names the paths that differ.
type manifest map[string]string

// readManifest runs find through the runner (sudo) because the extracted tree is root-owned. Every entry gets its mode, files also a sha256, links their target.
func readManifest(ctx context.Context, r runner, root string) (manifest, error) {
	m := manifest{}
	out, err := r.output(ctx, "find", root, "-mindepth", "1", "(", "-type", "f", "-o", "-type", "l", "-o", "-type", "d", ")",
		"-printf", "%m %y %p\t%l\n")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		head, link, _ := strings.Cut(line, "\t")
		f := strings.SplitN(head, " ", 3)
		if len(f) != 3 {
			continue
		}
		rel := strings.TrimPrefix(f[2], root)
		switch f[1] {
		case "l":
			m[rel] = f[0] + " l → " + link
		case "d":
			m[rel] = f[0] + " d"
		default:
			m[rel] = f[0] + " f"
		}
	}
	out, err = r.output(ctx, "find", root, "-type", "f", "-exec", "sha256sum", "{}", "+")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		sum, path, ok := strings.Cut(line, "  ")
		if !ok {
			continue
		}
		rel := strings.TrimPrefix(path, root)
		m[rel] = m[rel] + " " + sum
	}
	return m, nil
}

// diff lists where b differs from a: paths present on one side alone, and paths whose content, mode or link target differs. The result is sorted by path.
func (a manifest) diff(b manifest) []string {
	var out []string
	for p, va := range a {
		vb, ok := b[p]
		switch {
		case !ok:
			out = append(out, p+"  only in build 1")
		case va != vb:
			out = append(out, fmt.Sprintf("%s  build 1: %s  build 2: %s", p, short(va), short(vb)))
		}
	}
	for p := range b {
		if _, ok := a[p]; !ok {
			out = append(out, p+"  only in build 2")
		}
	}
	sort.Strings(out)
	return out
}

// short abbreviates the sha256 in a "mode f sha256" entry to 12 characters for display.
func short(v string) string {
	f := strings.Fields(v)
	if len(f) == 3 && len(f[2]) > 12 {
		f[2] = f[2][:12] + "…"
	}
	return strings.Join(f, " ")
}

// verify builds the same Dockerfile once more without cache and checks that the same PCRs come out. The static check (lint) says "may differ";
// this shows whether the builds do differ. Two equal builds on one host do not guarantee equality on another host, and a difference here means
// no host will match; the report names the paths responsible (measured: redis, the redis-server binary; haproxy, two apk db files).
// On a difference it returns an error. The first build's EIF and measurements are already in out/, and an Auditor could not reproduce them.
func (b Build) verify(ctx context.Context, r runner, appDir, appImage string, first manifest, firstPCR map[string]string) error {
	checkApp := appImage + "-verify"
	checkImage := b.ImageName + ":att-verify"
	fmt.Fprintf(r.log, "\n[verify 1/3] building %s again from scratch (--no-cache) → %s\n", appDir, checkApp)
	if err := r.run(ctx, "docker", "build", "-q", "--no-cache", "-t", checkApp, appDir); err != nil {
		return err
	}
	defer r.cmd(ctx, "docker", "rmi", "-f", checkApp, checkImage).Run()
	appCmd, err := r.imageCommand(ctx, checkApp)
	if err != nil {
		return err
	}
	if err := b.layerResponder(ctx, r, checkApp, checkImage, appCmd); err != nil {
		return err
	}
	fmt.Fprintf(r.log, "\n[verify 2/3] repacking the second build\n")
	second, err := normalize(ctx, r, checkImage, b.OutDir)
	if err != nil {
		return err
	}
	eif := filepath.Join(b.OutDir, b.ImageName+".verify.eif")
	defer os.Remove(eif)
	fmt.Fprintf(r.log, "\n[verify 3/3] measuring the second build\n")
	out, err := r.output(ctx, "nitro-cli", "build-enclave", "--docker-uri", checkImage, "--output-file", eif)
	if err != nil {
		return err
	}
	pcr, err := ReadPCR(out)
	if err != nil {
		return err
	}
	same := true
	for _, k := range []string{"PCR0", "PCR1", "PCR2"} {
		if pcr[k] != firstPCR[k] {
			same = false
			fmt.Fprintf(r.log, "%s differs: build 1 %s / build 2 %s\n", k, firstPCR[k], pcr[k])
		}
	}
	if same {
		fmt.Fprintln(r.log, cli.Green("✓ reproducible on this host")+": a second build from scratch gave the same PCR0·1·2")
		return nil
	}
	d := first.diff(second)
	fmt.Fprintf(r.log, "%s %d path(s) differ between the two builds (mode type sha256):\n", cli.Red("✗ not reproducible."), len(d))
	for i, line := range d {
		if i == 40 {
			fmt.Fprintf(r.log, "  … %d more\n", len(d)-i)
			break
		}
		fmt.Fprintln(r.log, "  "+line)
	}
	return fmt.Errorf("not reproducible: the same Dockerfile built twice on this host gave different measurements (%d path(s) above). fix the Dockerfile before handing the value on; the Auditor's rebuild will not match either", len(d))
}
