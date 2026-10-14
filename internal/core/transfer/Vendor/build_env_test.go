package vendor

import (
	"runtime"
	"strings"
	"testing"
)

// build_env: aws-nitro requires os, arch and go; arch is one the project builds for.
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
			t.Errorf("missing build_env.%s must be rejected on aws-nitro, got %v", field, err)
		}
	}
	c := cfgAWS()
	c.BuildEnv = BuildEnv{}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "build_env") {
		t.Errorf("an old config without build_env must be rejected on aws-nitro, got %v", err)
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
	// also on a platform where build_env is otherwise optional
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
		t.Fatal("current env is non-zero")
	}
	if s := e.String(); !strings.HasPrefix(s, runtime.GOOS+"/"+runtime.GOARCH+" "+runtime.Version()+" trimpath=") {
		t.Errorf("String() = %q", s)
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
