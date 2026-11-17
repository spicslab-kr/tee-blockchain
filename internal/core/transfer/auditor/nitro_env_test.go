package auditor

import (
	"encoding/json"
	"strings"
	"testing"
)

// The Nitro fields of build_env: optional, left out of the JSON when empty (configs and records written before them
// stay byte for byte the same), shown by String only when set.
func TestBuildEnv_NitroFields(t *testing.T) {
	plain := BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.26.1", Trimpath: true}
	b, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `{"os":"linux","arch":"amd64","go":"go1.26.1","trimpath":true}` {
		t.Errorf("without the Nitro fields the JSON must be as before, got %s", got)
	}
	if got := plain.String(); got != "linux/amd64 go1.26.1 trimpath=true" {
		t.Errorf("String() = %q", got)
	}
	full := plain
	full.AMIRelease, full.Docker, full.NitroCLI = "2023.12.20261109", "25.0.18-1.amzn2023.0.3", "1.5.0-0.amzn2023"
	b, _ = json.Marshal(full)
	var back BuildEnv
	if err := json.Unmarshal(b, &back); err != nil || back != full {
		t.Fatalf("round trip: %s -> %+v, %v", b, back, err)
	}
	if !strings.Contains(string(b), `"ami_release":"2023.12.20261109","docker":"25.0.18-1.amzn2023.0.3","nitro_cli":"1.5.0-0.amzn2023"`) {
		t.Errorf("JSON names: %s", b)
	}
	if got, want := full.String(), "linux/amd64 go1.26.1 trimpath=true ami_release=2023.12.20261109 docker=25.0.18-1.amzn2023.0.3 nitro_cli=1.5.0-0.amzn2023"; got != want {
		t.Errorf("String() = %q\nwant      %q", got, want)
	}
	cfg := cfgAWS()
	cfg.BuildEnv.AMIRelease, cfg.BuildEnv.Docker, cfg.BuildEnv.NitroCLI = full.AMIRelease, full.Docker, full.NitroCLI
	if err := cfg.Validate(); err != nil {
		t.Errorf("a config with the Nitro fields must validate: %v", err)
	}
}

// receive, on aws-nitro: the Amazon Linux release is compared (a config always declares one there; Validate).
func TestCheckBuildEnv_AMIRelease(t *testing.T) {
	want := BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.26.1", Trimpath: true, AMIRelease: "2023.12.20261109"}
	have := BuildEnv{OS: "linux", Arch: "amd64", Go: "go1.26.1", Trimpath: true, AMIRelease: "2023.12.20261109"}
	if w, err := CheckBuildEnv(want, have, false); err != nil || len(w) != 0 {
		t.Fatalf("same release: warnings %v, err %v", w, err)
	}

	have.AMIRelease = "2023.13.20261201"
	_, err := CheckBuildEnv(want, have, false)
	if err == nil || !strings.Contains(err.Error(), "ami_release: Vendor built on Amazon Linux 2023.12.20261109, this instance is 2023.13.20261201") {
		t.Fatalf("another release must be refused naming both: %v", err)
	}
	w, err := CheckBuildEnv(want, have, true)
	if err != nil || len(w) != 1 || !strings.Contains(w[0], "--ignore-env") {
		t.Errorf("with ignore the difference is a warning: %v, %v", w, err)
	}

	have.AMIRelease = "" // not Amazon Linux, or unreadable
	if _, err := CheckBuildEnv(want, have, false); err == nil || !strings.Contains(err.Error(), "not Amazon Linux 2023") {
		t.Errorf("an instance without a readable release must be refused: %v", err)
	}

	// TDX and SEV-SNP configs declare no release: nothing to compare, nothing to warn about
	want.AMIRelease, have.AMIRelease = "", ""
	if w, err := CheckBuildEnv(want, have, false); err != nil || len(w) != 0 {
		t.Errorf("no release on either side: %v, %v", w, err)
	}
}

// build --config: the versions themselves, after build installed them.
func TestCheckNitroTools(t *testing.T) {
	want := BuildEnv{Docker: "25.0.18-1.amzn2023.0.3", NitroCLI: "1.5.0-0.amzn2023"}
	if w, err := CheckNitroTools(want, "25.0.18-1.amzn2023.0.3", "1.5.0-0.amzn2023", false); err != nil || len(w) != 0 {
		t.Fatalf("same versions: %v, %v", w, err)
	}
	_, err := CheckNitroTools(want, "25.0.20-1.amzn2023.0.1", "1.5.0-0.amzn2023", false)
	if err == nil || !strings.Contains(err.Error(), "docker: Vendor built with 25.0.18-1.amzn2023.0.3, this instance installed 25.0.20-1.amzn2023.0.1") ||
		strings.Contains(err.Error(), "aws-nitro-enclaves-cli:") {
		t.Fatalf("another docker must be refused naming docker only: %v", err)
	}
	_, err = CheckNitroTools(want, "25.0.18-1.amzn2023.0.3", "1.4.2-0.amzn2023", false)
	if err == nil || !strings.Contains(err.Error(), "aws-nitro-enclaves-cli: Vendor built with 1.5.0-0.amzn2023, this instance installed 1.4.2-0.amzn2023") {
		t.Fatalf("another nitro-cli must be refused: %v", err)
	}
	w, err := CheckNitroTools(want, "25.0.20-1.amzn2023.0.1", "1.4.2-0.amzn2023", true)
	if err != nil || len(w) != 1 || !strings.Contains(w[0], "docker:") || !strings.Contains(w[0], "aws-nitro-enclaves-cli:") {
		t.Errorf("with ignore both differences are one warning: %v, %v", w, err)
	}
	_, err = CheckNitroTools(BuildEnv{}, "25.0.18-1.amzn2023.0.3", "1.5.0-0.amzn2023", false)
	if err == nil || !strings.Contains(err.Error(), "docker: the Vendor recorded no version") || !strings.Contains(err.Error(), "aws-nitro-enclaves-cli: the Vendor recorded no version") {
		t.Errorf("a version the Vendor did not record is a difference, not a pass: %v", err)
	}
}

// On aws-nitro the three fields are required (since 2026-11-17), so no config can skip the Auditor's two checks. The
// CVM formats have no docker or nitro-cli and do not need them.
func TestBuildConfig_NitroFieldsRequiredOnAWS(t *testing.T) {
	if err := cfgAWS().Validate(); err != nil {
		t.Fatalf("the full aws-nitro config must validate: %v", err)
	}
	for _, field := range []string{"ami_release", "docker", "nitro_cli"} {
		c := cfgAWS()
		switch field {
		case "ami_release":
			c.BuildEnv.AMIRelease = ""
		case "docker":
			c.BuildEnv.Docker = ""
		case "nitro_cli":
			c.BuildEnv.NitroCLI = ""
		}
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "build_env."+field+": required for aws-nitro") {
			t.Errorf("aws-nitro without build_env.%s must be rejected, got %v", field, err)
		}
	}
	if err := cfgTDX().Validate(); err != nil {
		t.Errorf("intel-tdx needs none of the three: %v", err)
	}
}
