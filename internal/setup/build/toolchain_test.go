package build

import (
	"runtime"
	"strings"
	"testing"
)

func TestRequiredToolchain(t *testing.T) {
	got := RequiredToolchain()
	if !strings.HasPrefix(got, "go1.") {
		t.Fatalf("RequiredToolchain() = %q, want a go1.x.y version string", got)
	}
	if got != strings.TrimSpace(got) || strings.ContainsAny(got, " \t\r\n") {
		t.Fatalf("RequiredToolchain() = %q contains whitespace", got)
	}
}

// The repo is built with the pinned toolchain; if this fails the machine is on the wrong Go and
// every measurement it produces is off. That is the point of the pin, so it must fail, not skip.
func TestCheckToolchainMatchesRunningGo(t *testing.T) {
	if err := CheckToolchain(); err != nil {
		t.Fatalf("this Go (%s) is not the pinned toolchain (%s); install it, or run tests with GOTOOLCHAIN=%s: %v",
			runtime.Version(), RequiredToolchain(), RequiredToolchain(), err)
	}
}

// The refusal is the same on every platform; its reason is the platform's own. On TDX and SEV-SNP the responder is
// not measured, so the message must not blame its bytes there.
func TestCheckToolchainFor_ReasonPerPlatform(t *testing.T) {
	cases := []struct {
		platform, must, mustNot string
	}{
		{"aws-nitro", "layered into the enclave image", "not measured"},
		{"intel-tdx", "att-ra-responder is not measured", "enclave image"},
		{"amd-sev-snp-initrd", "att-ra-responder is not measured", "enclave image"},
		{"amd-sev-snp-verity", "att-ra-responder is not measured", "enclave image"},
	}
	for _, c := range cases {
		err := checkToolchainFor("go1.26.2", "go1.26.1", c.platform)
		if err == nil {
			t.Fatalf("%s: want a refusal", c.platform)
		}
		msg := err.Error()
		if !strings.Contains(msg, c.must) || strings.Contains(msg, c.mustNot) {
			t.Errorf("%s: message %q must say %q and not %q", c.platform, msg, c.must, c.mustNot)
		}
		if !strings.Contains(msg, "GOTOOLCHAIN=go1.26.1") {
			t.Errorf("%s: message %q must give the rebuild command", c.platform, msg)
		}
	}
	if err := checkToolchainFor("go1.26.1", "go1.26.1", "intel-tdx"); err != nil {
		t.Fatalf("equal versions: unexpected error %v", err)
	}
}

func checkToolchain(have, want string) error { return checkToolchainFor(have, want, "") }

func TestCheckToolchainComparison(t *testing.T) {
	if err := checkToolchain("go1.26.1", "go1.26.1"); err != nil {
		t.Fatalf("equal versions: unexpected error %v", err)
	}
	cases := []struct{ have, want string }{
		{"go1.27.0", "go1.26.1"},
		{"go1.26.8", "go1.26.1"},
		{"go1.26.1-devel_abc", "go1.26.1"}, // dev-build suffix is a mismatch too
		{"devel go1.27-abc", "go1.26.1"},
	}
	for _, c := range cases {
		err := checkToolchain(c.have, c.want)
		if err == nil {
			t.Fatalf("checkToolchain(%q, %q): want error, got nil", c.have, c.want)
		}
		msg := err.Error()
		for _, needle := range []string{c.have, c.want, "GO_TOOLCHAIN", "GOTOOLCHAIN=" + c.want} {
			if !strings.Contains(msg, needle) {
				t.Errorf("checkToolchain(%q, %q) error %q does not mention %q", c.have, c.want, msg, needle)
			}
		}
	}
}
