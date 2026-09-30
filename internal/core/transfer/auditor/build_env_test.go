package auditor

import (
	"runtime"
	"strings"
	"testing"
)

// build_env — aws must declare os/arch/go; arch must be one the project builds for.
func TestBuildConfig_BuildEnvRequiredOnAWS(t *testing.T) {
	for _, field := range []string{"os", "arch", "go"} {
		c := cfgAWS()
		switch field {
		case "os":
			c.BuildEnv.OS = ""
		case "arch":
			c.BuildEnv.Arch = ""
		case "go":
			c.BuildEnv.Go = ""
		}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "build_env."+field) {
			t.Errorf("missing build_env.%s must be rejected on aws, got %v", field, err)
		}
	}
	c := cfgAWS()
	c.BuildEnv = BuildEnv{}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "build_env") {
		t.Errorf("an old config without build_env must be rejected on aws, got %v", err)
	}
	c = cfgAWS()
	c.BuildEnv.Trimpath = false
	if err := c.Validate(); err != nil {
		t.Errorf("trimpath=false is allowed (the Auditor warns): %v", err)
	}
	for _, other := range []BuildConfig{cfgGCP(), cfgAzure()} {
		if err := other.Validate(); err != nil {
			t.Errorf("%s: build_env is not required: %v", other.Platform, err)
		}
	}
}

func TestBuildConfig_BuildEnvArchMustBeKnown(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		c := cfgAWS()
		c.BuildEnv.Arch = arch
		if err := c.Validate(); err != nil {
			t.Errorf("%s must validate: %v", arch, err)
		}
	}
	for _, arch := range []string{"x86_64", "aarch64", "386", "riscv64"} {
		c := cfgAWS()
		c.BuildEnv.Arch = arch
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "build_env.arch: unknown") {
			t.Errorf("%s must be rejected, got %v", arch, err)
		}
	}
	c := cfgGCP()
	c.BuildEnv.Arch = "mips"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "build_env.arch") {
		t.Errorf("unknown arch must be rejected on gcp too, got %v", err)
	}
}

func TestCurrentBuildEnv(t *testing.T) {
	e := CurrentBuildEnv()
	if e.OS != runtime.GOOS || e.Arch != runtime.GOARCH || e.Go != runtime.Version() {
		t.Fatalf("got %+v, want %s/%s %s", e, runtime.GOOS, runtime.GOARCH, runtime.Version())
	}
	if e.OS == "" || e.Arch == "" || e.Go == "" {
		t.Fatalf("empty field in %+v", e)
	}
	if e.IsZero() {
		t.Fatal("current env is never zero")
	}
}

func TestBuildEnv_String(t *testing.T) {
	if got := (BuildEnv{OS: "linux", Arch: "arm64", Go: "go1.26.1", Trimpath: true}).String(); got != "linux/arm64 go1.26.1 trimpath=true" {
		t.Errorf("got %q", got)
	}
	if got := (BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.25.0"}).String(); got != "linux/amd64 go1.25.0 trimpath=false" {
		t.Errorf("got %q", got)
	}
}

// receive — the Vendor's declared env against this instance's. os/arch/go differing is an
// error (or a warning with --ignore-env); trimpath=false is only ever a warning.
func TestCheckBuildEnv(t *testing.T) {
	have := BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.26.1", Trimpath: true}

	t.Run("same env passes silently", func(t *testing.T) {
		w, err := CheckBuildEnv(have, have, false)
		if err != nil || len(w) != 0 {
			t.Fatalf("want no error, no warnings; got %v %v", err, w)
		}
	})

	t.Run("arch differs is an error naming both", func(t *testing.T) {
		want := have
		want.Arch = "arm64"
		_, err := CheckBuildEnv(want, have, false)
		if err == nil {
			t.Fatal("want error")
		}
		for _, s := range []string{"arch", "arm64", "amd64", "cannot produce the Vendor's measurements"} {
			if !strings.Contains(err.Error(), s) {
				t.Errorf("error %q must mention %q", err, s)
			}
		}
		if strings.Contains(err.Error(), "os:") || strings.Contains(err.Error(), "go:") {
			t.Errorf("only arch differs, but error says %q", err)
		}
	})

	t.Run("os differs is an error", func(t *testing.T) {
		want := have
		want.OS = "darwin"
		if _, err := CheckBuildEnv(want, have, false); err == nil || !strings.Contains(err.Error(), "os: Vendor built on darwin") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("go version differs is an error", func(t *testing.T) {
		want := have
		want.Go = "go1.25.3"
		if _, err := CheckBuildEnv(want, have, false); err == nil || !strings.Contains(err.Error(), "go: Vendor used go1.25.3") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("all three differ are all named", func(t *testing.T) {
		want := BuildEnv{OS: "darwin", Arch: "arm64", Go: "go1.25.3", Trimpath: true}
		_, err := CheckBuildEnv(want, have, false)
		if err == nil {
			t.Fatal("want error")
		}
		for _, s := range []string{"os:", "arch:", "go:"} {
			if !strings.Contains(err.Error(), s) {
				t.Errorf("error %q must mention %q", err, s)
			}
		}
	})

	t.Run("ignore downgrades to a warning", func(t *testing.T) {
		want := have
		want.Arch = "arm64"
		w, err := CheckBuildEnv(want, have, true)
		if err != nil {
			t.Fatalf("with ignore there must be no error: %v", err)
		}
		if len(w) != 1 || !strings.Contains(w[0], "arm64") || !strings.Contains(w[0], "ignore-env") {
			t.Fatalf("want one warning naming the arch and the flag, got %v", w)
		}
	})

	t.Run("trimpath=false warns but passes", func(t *testing.T) {
		want := have
		want.Trimpath = false
		w, err := CheckBuildEnv(want, have, false)
		if err != nil {
			t.Fatalf("trimpath is never an error: %v", err)
		}
		if len(w) != 1 || !strings.Contains(w[0], "trimpath") {
			t.Fatalf("want a trimpath warning, got %v", w)
		}
	})

	t.Run("this instance without trimpath is not the Vendor's problem", func(t *testing.T) {
		h := have
		h.Trimpath = false
		w, err := CheckBuildEnv(have, h, false)
		if err != nil || len(w) != 0 {
			t.Fatalf("want nothing, got %v %v", err, w)
		}
	})
}
