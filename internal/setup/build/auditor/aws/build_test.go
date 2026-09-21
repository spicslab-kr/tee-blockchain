package aws

import "testing"

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{"My App": "my-app", "example-ollama": "example-ollama", "///": "app", "a.b_c": "a.b_c"} {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadPCR(t *testing.T) {
	m, err := ReadPCR([]byte(`{"Measurements":{"HashAlgorithm":"Sha384","PCR0":"aa","PCR1":"bb","PCR2":"cc"}}`))
	if err != nil || m["PCR0"] != "aa" || m["PCR2"] != "cc" {
		t.Fatalf("ReadPCR: %v %v", m, err)
	}
	if _, err := ReadPCR([]byte("not json")); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
