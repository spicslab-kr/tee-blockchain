package vendor

import (
	"reflect"
	"strings"
	"testing"
)

// register --config: service/version/arch come from build-config.json; flags override with a warning.
func TestResolveRegistration(t *testing.T) {
	cfg := cfgAWS()
	cfg.Service, cfg.Version, cfg.BuildEnv.Arch = "nginx", "1.31.6", "arm64"

	t.Run("config only", func(t *testing.T) {
		svc, ver, arch, warns, err := ResolveRegistration(&cfg, "", "", "")
		if err != nil || svc != "nginx" || ver != "1.31.6" || arch != "arm64" || len(warns) != 0 {
			t.Fatalf("got %q %q %q %v %v", svc, ver, arch, warns, err)
		}
	})
	t.Run("flags only", func(t *testing.T) {
		svc, ver, arch, warns, err := ResolveRegistration(nil, "svc", "1.0.0", "amd64")
		if err != nil || svc != "svc" || ver != "1.0.0" || arch != "amd64" || len(warns) != 0 {
			t.Fatalf("got %q %q %q %v %v", svc, ver, arch, warns, err)
		}
	})
	t.Run("both, equal: no warning", func(t *testing.T) {
		svc, ver, arch, warns, err := ResolveRegistration(&cfg, "nginx", "1.31.6", "arm64")
		if err != nil || svc != "nginx" || ver != "1.31.6" || arch != "arm64" || len(warns) != 0 {
			t.Fatalf("got %q %q %q %v %v", svc, ver, arch, warns, err)
		}
	})
	t.Run("both, differing: the flag wins, one warning each", func(t *testing.T) {
		svc, ver, arch, warns, err := ResolveRegistration(&cfg, "nginx2", "", "amd64")
		if err != nil || svc != "nginx2" || ver != "1.31.6" || arch != "amd64" {
			t.Fatalf("got %q %q %q %v", svc, ver, arch, err)
		}
		want := []string{
			"--service nginx2 differs from build-config.json (nginx)",
			"--arch amd64 differs from build-config.json (arm64)",
		}
		if !reflect.DeepEqual(warns, want) {
			t.Errorf("warnings = %q, want %q", warns, want)
		}
	})
	t.Run("neither: error names both routes", func(t *testing.T) {
		_, _, _, _, err := ResolveRegistration(nil, "", "1.0.0", "amd64")
		if err == nil || !strings.Contains(err.Error(), "--service") || !strings.Contains(err.Error(), "--config") {
			t.Fatalf("want an error naming --service and --config, got %v", err)
		}
	})
}
