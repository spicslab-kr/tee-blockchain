package auditor

import (
	"errors"
	"fmt"
)

// ResolveRegistration returns the service, version and arch that register uses to name the build. With a
// build-config.json (cfg) the flags are optional and fill in from the config; a flag that is given wins,
// with one warning per flag that differs from the config. Without cfg, service and version are required
// and arch is used as given (the caller supplies its default).
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
