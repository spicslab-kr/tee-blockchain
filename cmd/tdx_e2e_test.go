// The recorded GCP TDX quote driven through the real binaries: ra-serve, ra-request and ra-parse with no chain, then
// audit and verify against a local anvil chain. The same shape as parse_e2e_test.go and flow_e2e_test.go for Nitro.
package cmd_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The values the recorded quote carries (same as internal/tee/parse/tdx/measure_test.go).
const (
	tdxMRTD  = "f21e6b48743fe14c2d090078ca1197f09ae4c99438677f2c79901ef80e2aeefe2f9a7eb6e3eb3b5047c4f1511731e5d3"
	tdxRTMR0 = "8847ec5d4441a3e41856f9de32c21676a788060c2746c565aee58b90a882fd427e9e45487dd992cd87eaf0b2fcdfee68"
	tdxRTMR1 = "8d55c85732cb5056c5826453f3b47de52c43f421f365af30a31aefc3ca027d7bfb5dba41c12b44a0797b07df0da4e9aa"
	tdxRTMR2 = "f44ab17ac6a021b3eb9207a512e7d9e2beab23d0ce8882cb5f5c0f90d236a2d065e70b0528c902a64dceaf9d3e81c8fc"
)

func tdxFixture(t *testing.T) (quote, nonce string) {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "tee", "parse", "tdx", "testdata")
	quote, _ = filepath.Abs(filepath.Join(dir, "gcp-c3-quote.bin"))
	nonce, _ = filepath.Abs(filepath.Join(dir, "gcp-c3-quote.nonce"))
	if _, err := os.Stat(quote); err != nil {
		t.Fatalf("TDX fixture missing: %v", err)
	}
	return quote, nonce
}

// writeTDXBuildMeasurements writes the measurements an Auditor would have measured, in the {"Measurements":{...}} shape
// ra-parse writes and audit --build-measurements reads.
func writeTDXBuildMeasurements(t *testing.T, dir, name, mrtd string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	b, _ := json.Marshal(map[string]any{"Format": "intel-tdx", "Measurements": map[string]string{
		"MRTD": mrtd, "RTMR0": tdxRTMR0, "RTMR1": tdxRTMR1, "RTMR2": tdxRTMR2}})
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The format name travels on the wire, so the user receives an intel-tdx document and the auditor's ra-parse picks
// the TDX parser. The fixture extractor ignores the request nonce, so ra-parse is given the quote's own.
func TestCmd_E2E_TDX_ServeRequestParse(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	quote, nonce := tdxFixture(t)
	bins := t.TempDir()
	vendor := build(t, bins, "./cmd/vendor")
	user := build(t, bins, "./cmd/user")
	auditor := build(t, bins, "./cmd/auditor")

	addr, stop := startServeAs(t, vendor, quote, "intel-tdx")
	defer stop()

	work := t.TempDir()
	docPath := filepath.Join(work, "quote.bin")
	if out, err := exec.Command(user, "ra-request", "--tee-addr", addr, "--out-file", docPath, "--format", "intel-tdx", "--timeout", "10s").CombinedOutput(); err != nil {
		t.Fatalf("ra-request: %v\n%s", err, out)
	}
	// asking for one format and receiving another is refused before anything is parsed
	if out, err := exec.Command(user, "ra-request", "--tee-addr", addr, "--out-file", filepath.Join(work, "x"), "--format", "aws-nitro", "--timeout", "10s").CombinedOutput(); err == nil {
		t.Fatalf("an intel-tdx document must not be accepted as aws-nitro:\n%s", out)
	}

	measPath := docPath + ".measurements.json"
	out, err := exec.Command(auditor, "ra-parse", "--doc", docPath, "--format", "intel-tdx", "--nonce", nonce, "--out-file", measPath).CombinedOutput()
	if err != nil {
		t.Fatalf("ra-parse: %v\n%s", err, out)
	}
	for _, want := range []string{"signature  ok", "nonce      ok", "debug      false", "timestamp  none in the document"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("ra-parse output lacks %q:\n%s", want, out)
		}
	}
	raw, err := os.ReadFile(measPath)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Format       string
		Measurements map[string]string
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"MRTD": tdxMRTD, "RTMR0": tdxRTMR0, "RTMR1": tdxRTMR1, "RTMR2": tdxRTMR2}
	if got.Format != "intel-tdx" || len(got.Measurements) != len(want) {
		t.Fatalf("measurements.json = %s", raw)
	}
	for k, v := range want {
		if got.Measurements[k] != v {
			t.Errorf("%s = %s, want %s", k, got.Measurements[k], v)
		}
	}

	// the same quote judged as aws-nitro is malformed, not silently accepted
	if out, err := exec.Command(auditor, "ra-parse", "--doc", docPath, "--format", "aws-nitro", "--out-file", filepath.Join(work, "y")).CombinedOutput(); err == nil {
		t.Fatalf("a TDX quote must not parse as aws-nitro:\n%s", out)
	}
}

// Auditor audits a TDX guest and registers MRTD and RTMR0-2 under intel-tdx; the User verifies the same guest and is
// told it is trusted. A built value that differs from the attested one is not registered. Skipped if anvil is not on PATH.
func TestRun_E2E_TDX_AuditorUser(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stopAnvil := startAnvil(t)
	defer stopAnvil()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
	auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)

	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	userBin := buildBin(t, bins, "./cmd/user")
	registryAddr := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)

	quote, nonce := tdxFixture(t)
	teeAddr, stopServe := startServeAs(t, vendorBin, quote, "intel-tdx")
	defer stopServe()

	// Before anything is registered, the User finds nothing.
	uOut, err := exec.Command(userBin, "verify", "--tee-addr", teeAddr, "--format", "intel-tdx", "--rpc", rpc, "--registry", registryAddr,
		"--nonce", nonce, "--out-dir", t.TempDir()).CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 || !strings.Contains(string(uOut), "nobody has registered") {
		t.Fatalf("verify before registration must exit 1 with 'nobody has registered': %v\n%s", err, uOut)
	}

	work := t.TempDir()
	badBuild := writeTDXBuildMeasurements(t, work, "build-bad.json", strings.Repeat("ab", 48))
	aBad, err := exec.Command(auditorBin, "audit", "--tee-addr", teeAddr, "--format", "intel-tdx", "--build-measurements", badBuild,
		"--rpc", rpc, "--registry", registryAddr, "--signing-key-file", auditorFile, "--service", "tdx-e2e", "--version", "1.0.0",
		"--nonce", nonce, "--out-dir", filepath.Join(work, "bad")).CombinedOutput()
	if err == nil || !strings.Contains(string(aBad), "not registering") || !strings.Contains(string(aBad), "MRTD") {
		t.Fatalf("a different MRTD must stop the registration and be named:\n%s", aBad)
	}

	goodBuild := writeTDXBuildMeasurements(t, work, "build-good.json", tdxMRTD)
	aOut, err := exec.Command(auditorBin, "audit", "--tee-addr", teeAddr, "--format", "intel-tdx", "--build-measurements", goodBuild,
		"--rpc", rpc, "--registry", registryAddr, "--signing-key-file", auditorFile, "--service", "tdx-e2e", "--version", "1.0.0",
		"--nonce", nonce, "--out-dir", filepath.Join(work, "good")).CombinedOutput()
	if err != nil {
		t.Fatalf("auditor audit: %v\n%s", err, aOut)
	}
	if !strings.Contains(string(aOut), "identical") || !strings.Contains(string(aOut), "PASS") {
		t.Fatalf("audit must print both the check PASS and the self-check PASS:\n%s", aOut)
	}

	uOut, err = exec.Command(userBin, "verify", "--tee-addr", teeAddr, "--format", "intel-tdx", "--rpc", rpc, "--registry", registryAddr,
		"--nonce", nonce, "--out-dir", t.TempDir()).CombinedOutput()
	if err != nil {
		t.Fatalf("user verify: %v\n%s", err, uOut)
	}
	if !strings.Contains(string(uOut), "trusted:") || !strings.Contains(string(uOut), "tdx-e2e 1.0.0") {
		t.Fatalf("verify must say the TDX guest is trusted:\n%s", uOut)
	}

	// The registration is under intel-tdx: looking it up as aws-nitro finds nothing.
	meas := filepath.Join(work, "good", "parsed.json")
	lOut, err := exec.Command(userBin, "lookup", "--rpc", rpc, "--registry", registryAddr, "--measurements", meas).CombinedOutput()
	if err != nil || !strings.Contains(string(lOut), "nothing registered") {
		t.Fatalf("an intel-tdx registration must not be found under aws-nitro: %v\n%s", err, lOut)
	}
	lOut, err = exec.Command(userBin, "lookup", "--rpc", rpc, "--registry", registryAddr, "--measurements", meas, "--platform", "intel-tdx").CombinedOutput()
	if err != nil || !strings.Contains(string(lOut), "intel-tdx") {
		t.Fatalf("lookup --platform intel-tdx must find it: %v\n%s", err, lOut)
	}
}
