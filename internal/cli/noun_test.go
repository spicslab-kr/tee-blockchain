package cli

import "testing"

func TestTEENoun(t *testing.T) {
	for format, want := range map[string]string{
		"":                   "enclave",
		"aws-nitro":          "enclave",
		"intel-tdx":          "guest",
		"amd-sev-snp-initrd": "guest",
		"amd-sev-snp-verity": "guest",
	} {
		if got := TEENoun(format); got != want {
			t.Errorf("TEENoun(%q) = %q, want %q", format, got, want)
		}
	}
}
