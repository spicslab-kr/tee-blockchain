package vendor

import (
	"errors"
	"fmt"
)

// ResolveRegistration — what register needs to name the build it registers (service, version,
// arch), from build-config.json when the Vendor handed one over, or from the flags typed by hand.
// The Vendor already said all three in the config, so with cfg the flags are optional; a flag that
// is given anyway wins, with one warning line per flag that differs from the config. Without cfg,
// service and version are required and arch is taken as given (the caller defaulted it).
func ResolveRegistration(cfg *BuildConfig, service, version, arch string) (svc, ver, archOut string, warnings []string, err error) {
	if cfg == nil {
		if service == "" || version == "" {
			return "", "", "", nil, errors.New("--service and --version are required (or --config <build-config.json>, which carries them)")
		}
		return service, version, arch, nil, nil
	}
	pick := func(flag, flagValue, cfgValue string) string {
		if flagValue == "" {
			return cfgValue
		}
		if flagValue != cfgValue {
			warnings = append(warnings, fmt.Sprintf("--%s %s differs from build-config.json (%s)", flag, flagValue, cfgValue))
		}
		return flagValue
	}
	svc = pick("service", service, cfg.Service)
	ver = pick("version", version, cfg.Version)
	archOut = pick("arch", arch, cfg.BuildEnv.Arch)
	return svc, ver, archOut, warnings, nil
}
