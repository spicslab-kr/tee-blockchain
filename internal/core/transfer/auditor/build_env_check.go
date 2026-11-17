package auditor

import (
	"errors"
	"strings"
)

// CheckBuildEnv compares the environment the Vendor declared (want) with this instance's (have). OS,
// architecture and Go version each change the measurements, so a difference in any of them is an error:
// this instance cannot produce the Vendor's PCRs, and a rebuild here would look like a lying Vendor. With
// ignore set, the error becomes a warning. A Vendor build without -trimpath always gives a warning, since
// its measurements may depend on the checkout path.
//
// When the Vendor declared an Amazon Linux release (required on aws-nitro, see Validate), have.AMIRelease must be the
// same: the release decides which docker and nitro-cli dnf will install here. That is a proxy, not a guarantee (see
// BuildEnv.AMIRelease); the versions themselves are compared by CheckNitroTools in build, after they are installed.
func CheckBuildEnv(want, have BuildEnv, ignore bool) (warnings []string, err error) {
	var diffs []string
	if want.OS != have.OS {
		diffs = append(diffs, "os: Vendor built on "+want.OS+", this instance is "+have.OS)
	}
	if want.Arch != have.Arch {
		diffs = append(diffs, "arch: Vendor built for "+want.Arch+", this instance is "+have.Arch)
	}
	if want.Go != have.Go {
		diffs = append(diffs, "go: Vendor used "+want.Go+", this instance has "+have.Go)
	}
	if want.AMIRelease != "" && want.AMIRelease != have.AMIRelease {
		mine := have.AMIRelease
		if mine == "" {
			mine = "not Amazon Linux 2023, or its release could not be read"
		}
		diffs = append(diffs, "ami_release: Vendor built on Amazon Linux "+want.AMIRelease+", this instance is "+mine+
			" (each release installs its own docker and nitro-cli versions)")
	}
	if !want.Trimpath {
		warnings = append(warnings, "Vendor built without -trimpath: its measurements may depend on its checkout path")
	}
	if len(diffs) == 0 {
		return warnings, nil
	}
	msg := "build env differs from the Vendor's (" + strings.Join(diffs, "; ") +
		"): this instance cannot produce the Vendor's measurements"
	if ignore {
		return append(warnings, msg+" (continuing because of --ignore-env)"), nil
	}
	return warnings, errors.New(msg)
}

// CheckNitroTools compares the docker and aws-nitro-enclaves-cli versions the Vendor built with (want.Docker and
// want.NitroCLI, recorded by att-vendor send and required on aws-nitro) with the ones build has just installed here.
// This is the real check behind the ami_release proxy: a difference means this build will not give the Vendor's
// measurements (PCR1 follows nitro-cli's kernel and init, the image follows docker). A version the Vendor did not
// record counts as a difference (a validated aws-nitro config always has both). With ignore set, the error becomes a
// warning.
func CheckNitroTools(want BuildEnv, haveDocker, haveNitroCLI string, ignore bool) (warnings []string, err error) {
	var diffs []string
	for _, c := range []struct{ name, want, have string }{
		{"docker", want.Docker, haveDocker},
		{"aws-nitro-enclaves-cli", want.NitroCLI, haveNitroCLI},
	} {
		switch {
		case c.want == "":
			diffs = append(diffs, c.name+": the Vendor recorded no version, this instance installed "+c.have)
		case c.want != c.have:
			diffs = append(diffs, c.name+": Vendor built with "+c.want+", this instance installed "+c.have)
		}
	}
	if len(diffs) == 0 {
		return nil, nil
	}
	msg := "build tools differ from the Vendor's (" + strings.Join(diffs, "; ") +
		"): this build cannot produce the Vendor's measurements"
	if ignore {
		return []string{msg + " (continuing because of --ignore-env)"}, nil
	}
	return nil, errors.New(msg)
}
