// Package tdx launches a built initrd on an Intel TDX guest and reads back what the boot measured. It is the
// role-independent core; internal/setup/launch/{Vendor,auditor}/tdx are thin adapters.
//
// On a confidential VM, booting the VM is the launch. It takes two runs with a reboot between them
// (docs/plan/cvm-port.md, "Phase 3 design"):
//
//  1. Install: put the initrd in /boot and write a GRUB entry made from the stock one, as the default (grub.go, install.go).
//  2. Check, after the reboot: a fresh quote, verified; the CC event log replayed against its RTMRs; the initrd GRUB
//     loaded must have the build's SHA-384; the boot marker must be there. Then MRTD and RTMR0-2 are written out as the
//     measurements to register (check.go, eventlog.go).
package tdx

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// The stock entry is the first one GRUB's 10_linux writes on Ubuntu, titled 'Ubuntu'. The recordings used it as the
// template (docs/plan/app-fixture-recording.md); internal/setup/build/cpio/testdata/make-grub-entry.sh did this with sed.
var (
	stockStart  = regexp.MustCompile(`^menuentry 'Ubuntu'.*\{\s*$`)
	initrdName  = regexp.MustCompile(`initrd\.img-[^\s]*`)
	linuxLine   = regexp.MustCompile(`^\s*linux\s`)
	initrdLine  = regexp.MustCompile(`^\s*initrd\s`)
	validID     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	validInitrd = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// StockEntry returns the stock 'Ubuntu' menuentry block of a grub.cfg, from its first line to the closing brace.
func StockEntry(grubCfg string) (string, error) {
	var out []string
	in := false
	for _, line := range strings.Split(grubCfg, "\n") {
		if !in && stockStart.MatchString(line) {
			in = true
		}
		if in {
			out = append(out, line)
			if line == "}" {
				return strings.Join(out, "\n") + "\n", nil
			}
		}
	}
	return "", errors.New("grub.cfg has no menuentry 'Ubuntu' block (is this an Ubuntu guest?)")
}

// Entry makes our entry from the stock one, changing only the title (to id, with --id id), the initrd file name and,
// if given, appending kernel arguments to the linux line. Everything else - search, insmod, the kernel, its command
// line - stays the stock entry's, because every GRUB command and the command line are measured into RTMR2: the fewer
// differences from the stock boot, the fewer surprises in the measurement.
//
// A stock entry with no initrd line is refused: with GRUB_FORCE_PARTUUID set, Ubuntu's cloud images boot without an
// initrd, and an application placed in one would never be loaded or measured (docs/build/tdx.md).
func Entry(stock, id, initrdFile, extraArgs string) (string, error) {
	if !validID.MatchString(id) {
		return "", fmt.Errorf("entry id %q: lower-case letters, digits and '-', at most 32", id)
	}
	if !validInitrd.MatchString(initrdFile) {
		return "", fmt.Errorf("initrd file name %q: a plain file name in /boot", initrdFile)
	}
	if strings.ContainsAny(extraArgs, "\n'\"{};$`\\") {
		return "", fmt.Errorf("extra kernel arguments %q: no quotes, braces, newlines or shell characters", extraArgs)
	}
	lines := strings.Split(strings.TrimRight(stock, "\n"), "\n")
	if len(lines) < 2 || !stockStart.MatchString(lines[0]) || lines[len(lines)-1] != "}" {
		return "", errors.New("not a stock 'Ubuntu' menuentry block")
	}
	lines[0] = fmt.Sprintf("menuentry '%s' --id %s {", id, id)
	var linux, initrd int
	for i, l := range lines[1:] {
		switch {
		case linuxLine.MatchString(l):
			linux++
			if extraArgs != "" {
				lines[i+1] = strings.TrimRight(l, " \t") + " " + extraArgs
			}
		case initrdLine.MatchString(l):
			initrd++
			if !initrdName.MatchString(l) {
				return "", fmt.Errorf("the stock initrd line has no initrd.img-<version> to replace: %q", strings.TrimSpace(l))
			}
			lines[i+1] = initrdName.ReplaceAllLiteralString(l, initrdFile)
		}
	}
	if linux == 0 {
		return "", errors.New("the stock entry has no linux line")
	}
	if initrd == 0 {
		return "", errors.New("the stock entry loads no initrd: GRUB_FORCE_PARTUUID is probably set, so the application's initrd would never be loaded. Comment it out in /etc/default/grub.d/40-force-partuuid.cfg and run update-grub (docs/build/tdx.md)")
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// grubScript is /etc/grub.d/41_<id>: update-grub runs every file there and copies its output into grub.cfg. The two
// header lines make it print the rest of itself, as make-grub-entry.sh wrote it.
func grubScript(entry string) string {
	return "#!/bin/sh\nexec tail -n +3 $0\n" + entry
}
