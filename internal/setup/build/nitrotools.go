// What decides the docker and nitro-cli versions on a Nitro parent instance, and the versions themselves.
//
// build installs docker and aws-nitro-enclaves-cli with dnf and no version (aws.Build.Run). PCR1 follows nitro-cli's
// kernel and init files, and docker builds the image, so the Vendor and the Auditor need the same two packages.
// Amazon Linux 2023 locks each release to one versioned package repository by default ("deterministic upgrades"), so
// the release an instance was installed from, e.g. 2023.12.20261109, decides which versions dnf gives it.
//
// The release is a proxy, not a guarantee. It stops naming the versions as soon as what dnf installs from changes
// without the release changing with it, or changes after the build: `releasever=latest` in /etc/dnf/vars, a
// `dnf update` or `dnf upgrade --releasever=…` run between build and send, a package from another repository, or
// docker installed by hand. So the release is compared at receive, before anything is installed, and the versions
// themselves (NitroTools) are compared by the Auditor's build right after it installed them; that is the real check.

package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// OSReleasePath is where AmazonLinuxRelease reads the release from.
const OSReleasePath = "/etc/os-release"

// NitroPackages are the two packages whose versions the Vendor and the Auditor must share, in NitroTools order.
var NitroPackages = []string{"docker", "aws-nitro-enclaves-cli"}

var alRelease = regexp.MustCompile(`\b(20[0-9]{2}\.[0-9]+\.[0-9]{8})\b`)

// AmazonLinuxRelease returns the Amazon Linux 2023 release this machine runs, e.g. "2023.12.20261109", from PRETTY_NAME
// in /etc/os-release ("Amazon Linux 2023.12.20261109" on the instances of 2026-11-12 and 2026-11-16). Not the AMI ID,
// which differs per region for the same image. A machine that is not Amazon Linux 2023 gives an error.
func AmazonLinuxRelease() (string, error) {
	b, err := os.ReadFile(OSReleasePath)
	if err != nil {
		return "", err
	}
	return ParseAmazonLinuxRelease(b)
}

// ParseAmazonLinuxRelease reads the release out of the contents of /etc/os-release.
func ParseAmazonLinuxRelease(osRelease []byte) (string, error) {
	var id, pretty string
	for _, line := range strings.Split(string(osRelease), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			id = v
		case "PRETTY_NAME":
			pretty = v
		}
	}
	if id != "amzn" {
		return "", fmt.Errorf("%s: not Amazon Linux (ID=%q)", OSReleasePath, id)
	}
	m := alRelease.FindStringSubmatch(pretty)
	if m == nil {
		return "", fmt.Errorf("%s: no release like 2023.12.20261109 in PRETTY_NAME %q", OSReleasePath, pretty)
	}
	return m[1], nil
}

// NitroTools is the version-release of the two packages build installs, as rpm reports them without the architecture,
// e.g. Docker "25.0.18-1.amzn2023.0.3" and NitroCLI "1.5.0-0.amzn2023" (the instances of 2026-11-12 and 2026-11-16).
// These are the package versions; `docker --version` reports another patch number (25.0.14) for the same package.
type NitroTools struct {
	Docker   string
	NitroCLI string
}

// String is one line for the build log.
func (t NitroTools) String() string {
	return "tools: docker " + t.Docker + " · aws-nitro-enclaves-cli " + t.NitroCLI
}

// rpmQuery runs rpm for the packages and returns its standard output; a variable so tests need no rpm.
var rpmQuery = func(ctx context.Context, pkgs ...string) ([]byte, error) {
	args := append([]string{"-q", "--queryformat", "%{VERSION}-%{RELEASE}\\n"}, pkgs...)
	var stderr bytes.Buffer
	c := exec.CommandContext(ctx, "rpm", args...)
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("rpm -q %s: %v %s", strings.Join(pkgs, " "), err, strings.TrimSpace(stderr.String()))
	}
	// rpm exits 1 when a package is not installed and says so on standard output; ParseRPMVersions names it
	return out, nil
}

// ReadNitroTools asks rpm for the installed versions of docker and aws-nitro-enclaves-cli.
func ReadNitroTools(ctx context.Context) (NitroTools, error) {
	out, err := rpmQuery(ctx, NitroPackages...)
	if err != nil {
		return NitroTools{}, err
	}
	return ParseRPMVersions(out)
}

// ParseRPMVersions reads `rpm -q --queryformat '%{VERSION}-%{RELEASE}\n' docker aws-nitro-enclaves-cli`: one line per
// package in that order, or "package <name> is not installed" for a missing one.
func ParseRPMVersions(out []byte) (NitroTools, error) {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(NitroPackages) {
		return NitroTools{}, fmt.Errorf("rpm -q %s: want %d lines, got %q", strings.Join(NitroPackages, " "), len(NitroPackages), out)
	}
	var missing []string
	for i, l := range lines {
		if strings.HasPrefix(l, "package ") || strings.TrimSpace(l) == "" || strings.ContainsAny(l, " \t") {
			missing = append(missing, NitroPackages[i])
		}
	}
	if len(missing) > 0 {
		return NitroTools{}, errors.New("not installed: " + strings.Join(missing, ", ") + " (build installs them; run it first)")
	}
	return NitroTools{Docker: lines[0], NitroCLI: lines[1]}, nil
}
