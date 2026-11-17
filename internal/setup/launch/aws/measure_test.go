package aws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nitroOut = `{
  "Measurements": {
    "HashAlgorithm": "Sha512_48 { .. }",
    "PCR0": "AA00",
    "PCR1": "BB11",
    "PCR2": "CC22"
  }
}`

func TestReadPCR(t *testing.T) {
	pcr, err := readPCR([]byte(nitroOut))
	if err != nil {
		t.Fatal(err)
	}
	if pcr["PCR0"] != "aa00" || pcr["PCR1"] != "bb11" || pcr["PCR2"] != "cc22" || len(pcr) != 3 {
		t.Fatalf("pcr = %v", pcr)
	}
	if _, err := readPCR([]byte(`{"Measurements":{"PCR0":"aa"}}`)); err == nil || !strings.Contains(err.Error(), "no PCR1") {
		t.Errorf("missing PCR must be an error, got %v", err)
	}
}

func TestWriteMeasurements_LeavesBuildFileAlone(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "measurements.json")
	if err := os.WriteFile(p, []byte(nitroOut), 0o644); err != nil {
		t.Fatal(err)
	}
	pcr := map[string]string{"PCR0": "aa00", "PCR1": "bb11", "PCR2": "cc22"}
	wrote, err := writeMeasurements(p, pcr)
	if err != nil || wrote {
		t.Fatalf("same values: wrote=%v err=%v: build's file must be kept", wrote, err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != nitroOut {
		t.Errorf("build's file was rewritten")
	}

	pcr["PCR2"] = "dd33"
	wrote, err = writeMeasurements(p, pcr)
	if err != nil || !wrote {
		t.Fatalf("different values: wrote=%v err=%v", wrote, err)
	}
	got, _ := readPCR(mustRead(t, p))
	if got["PCR2"] != "dd33" {
		t.Errorf("rewritten file = %v", got)
	}
}

func TestCheckAgainst(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "claimed.json")
	if err := os.WriteFile(p, []byte(nitroOut), 0o644); err != nil {
		t.Fatal(err)
	}
	same := map[string]string{"PCR0": "aa00", "PCR1": "bb11", "PCR2": "cc22"}
	if r, err := checkAgainst(same, p); err != nil || len(r) != 0 {
		t.Fatalf("same: %v %v", r, err)
	}
	diff := map[string]string{"PCR0": "aa00", "PCR1": "bb11", "PCR2": "ffffffffffffffffffff"}
	r, err := checkAgainst(diff, p)
	if err != nil || len(r) != 1 || !strings.HasPrefix(r[0], "PCR2: ") {
		t.Fatalf("diff: %v %v", r, err)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
