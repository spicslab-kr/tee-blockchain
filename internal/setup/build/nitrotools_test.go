package build

import (
	"context"
	"strings"
	"testing"
)

// The PRETTY_NAME line is the one recorded on the Nitro instances of 2026-11-12 and 2026-11-16
// (docs/build/toolchain.md); the other lines were written for this test and were not copied from an instance.
const al2023OSRelease = `NAME="Amazon Linux"
VERSION="2023"
ID="amzn"
ID_LIKE="fedora"
VERSION_ID="2023"
PLATFORM_ID="platform:al2023"
PRETTY_NAME="Amazon Linux 2023.12.20261109"
ANSI_COLOR="0;33"
`

func TestParseAmazonLinuxRelease(t *testing.T) {
	got, err := ParseAmazonLinuxRelease([]byte(al2023OSRelease))
	if err != nil || got != "2023.12.20261109" {
		t.Fatalf("got %q, %v; want 2023.12.20261109", got, err)
	}
	ubuntu := "NAME=\"Ubuntu\"\nID=ubuntu\nPRETTY_NAME=\"Ubuntu 24.04.3 LTS\"\n"
	if _, err := ParseAmazonLinuxRelease([]byte(ubuntu)); err == nil || !strings.Contains(err.Error(), "not Amazon Linux") {
		t.Errorf("Ubuntu should be refused as not Amazon Linux: %v", err)
	}
	noRelease := strings.Replace(al2023OSRelease, "Amazon Linux 2023.12.20261109", "Amazon Linux 2023", 1)
	if _, err := ParseAmazonLinuxRelease([]byte(noRelease)); err == nil || !strings.Contains(err.Error(), "no release like") {
		t.Errorf("a PRETTY_NAME without a release should be an error: %v", err)
	}
}

func TestParseRPMVersions(t *testing.T) {
	got, err := ParseRPMVersions([]byte("25.0.18-1.amzn2023.0.3\n1.5.0-0.amzn2023\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := (NitroTools{Docker: "25.0.18-1.amzn2023.0.3", NitroCLI: "1.5.0-0.amzn2023"}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if s := got.String(); s != "tools: docker 25.0.18-1.amzn2023.0.3 · aws-nitro-enclaves-cli 1.5.0-0.amzn2023" {
		t.Errorf("String() = %q", s)
	}
	_, err = ParseRPMVersions([]byte("25.0.18-1.amzn2023.0.3\npackage aws-nitro-enclaves-cli is not installed\n"))
	if err == nil || !strings.Contains(err.Error(), "not installed: aws-nitro-enclaves-cli") || strings.Contains(err.Error(), "docker") {
		t.Errorf("a missing nitro-cli should be named alone: %v", err)
	}
	if _, err := ParseRPMVersions([]byte("25.0.18-1.amzn2023.0.3\n")); err == nil {
		t.Error("one line for two packages should be an error")
	}
}

func TestReadNitroTools_AsksRPMForBothPackages(t *testing.T) {
	saved := rpmQuery
	defer func() { rpmQuery = saved }()
	var asked []string
	rpmQuery = func(_ context.Context, pkgs ...string) ([]byte, error) {
		asked = pkgs
		return []byte("25.0.18-1.amzn2023.0.3\n1.5.0-0.amzn2023\n"), nil
	}
	got, err := ReadNitroTools(context.Background())
	if err != nil || got.Docker != "25.0.18-1.amzn2023.0.3" || got.NitroCLI != "1.5.0-0.amzn2023" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if strings.Join(asked, " ") != "docker aws-nitro-enclaves-cli" {
		t.Errorf("asked rpm for %q", asked)
	}
}
