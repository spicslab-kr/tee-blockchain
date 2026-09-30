package auditor

import (
	"errors"
	"strings"
)

// CheckBuildEnv — compares the environment the Vendor declared (want) with the one this
// instance has (have). OS, architecture and Go version each change the measurements, so a
// difference in any of them is an error: this instance cannot produce the Vendor's PCRs, and a
// rebuild here would look exactly like a lying Vendor. With ignore set, those become warnings.
// A Vendor that built without -trimpath is only ever a warning — its measurements may then
// depend on its checkout path, which no Auditor can be expected to match.
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
	if !want.Trimpath {
		warnings = append(warnings, "Vendor built without -trimpath: its measurements may depend on its checkout path")
	}
	if len(diffs) == 0 {
		return warnings, nil
	}
	msg := "build env differs from the Vendor's (" + strings.Join(diffs, "; ") +
		"): this instance cannot produce the Vendor's measurements"
	if ignore {
		return append(warnings, msg+" — continuing because of --ignore-env"), nil
	}
	return warnings, errors.New(msg)
}
