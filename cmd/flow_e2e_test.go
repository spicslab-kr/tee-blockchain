// run through the real CLI: what a person used to type by hand as separate steps (request, parse, register, lookup, compare), bundled into one command.
// One registry = one approver, so the Auditor and the Vendor each register on their own contract.
// Skipped if anvil is not on PATH.
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The PCRs fixture-my-web.cbor actually carries: the values nitro-cli build-enclave produced then (same as internal/tee/parse/aws/measure_test.go).
const (
	fixturePCR0 = "a0099c1cd2af380adf19601ad09aa49380fc8c3c78e875ac9fe958fc6a836a8d3c30ca5ff5d66c9dc7293cd30ce67fb1"
	fixturePCR1 = "4b4d5b3661b3efc12920900c80e126e4ce783c522de6c02a2a5bf7af3a2b9327b86776f188e4be1c1c404a129dbda493"
	fixturePCR2 = "62f677d0dea159ce42932fa7cc14b885d53d295136f74347a8572ba5f48a36e82485055108bd4f1fa2a3302ab64424cf"
)

// writeBuildMeasurements: the same nitro-cli format that build leaves in out/measurements.json.
func writeBuildMeasurements(t *testing.T, dir, name, pcr0, pcr1, pcr2 string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	body := `{"Measurements":{"HashAlgorithm":"Sha384 { ... }","PCR0":"` + pcr0 + `","PCR1":"` + pcr1 + `","PCR2":"` + pcr2 + `"}}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// lastRecordKey: recordKeyLineRe may match more than once (the service/version inference log). The record key register actually
// registered is always after that, the last match in the output.
func lastRecordKey(out []byte) []byte {
	m := recordKeyLineRe.FindAllSubmatch(out, -1)
	if m == nil {
		return nil
	}
	return m[len(m)-1][1]
}

func deployFor(t *testing.T, bin, rpc, deployerFile, approver string) string {
	t.Helper()
	out, err := exec.Command(bin, "registry-deploy", "--rpc", rpc, "--deployer-key-file", deployerFile, "--approver", approver).CombinedOutput()
	if err != nil {
		t.Fatalf("registry-deploy: %v\n%s", err, out)
	}
	m := deployedAtRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("registry address not found:\n%s", out)
	}
	return string(m[1])
}

func TestRun_E2E_AuditorVendorUser(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stop := startAnvil(t)
	defer stop()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)
	auditorFile := writeKeyFile(t, keys, "auditor.key", auditorKey)
	vendorFile := writeKeyFile(t, keys, "vendor.key", vendorKey)

	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	userBin := buildBin(t, bins, "./cmd/user")

	auditorRegistryAddr := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)
	vendorRegistryAddr := deployFor(t, vendorBin, rpc, deployerFile, vendorAddr)

	fixture := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.cbor")
	fixtureNonce := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.nonce")
	teeAddr, stopServe := startServe(t, vendorBin, fixture)
	defer stopServe()

	// The fixture responder is not bound to the nonce sent (FixtureExtractor.Extract ignores the nonce), so ra-parse
	// must be given the nonce the fixture actually carries, not the default (the nonce ra-request just sent).
	work := t.TempDir()
	goodBuild := writeBuildMeasurements(t, work, "build-good.json", fixturePCR0, fixturePCR1, fixturePCR2)
	badBuild := writeBuildMeasurements(t, work, "build-bad.json", strings.Repeat("ab", 48), fixturePCR1, fixturePCR2)

	// Auditor, check FAIL: if the value I built (a fake) differs from what the TEE attests, it does not register and fails.
	aBad := exec.Command(auditorBin, "audit",
		"--tee-addr", teeAddr, "--build-measurements", badBuild, "--rpc", rpc, "--registry", auditorRegistryAddr, "--signing-key-file", auditorFile,
		"--service", "run-e2e", "--version", "1.0.0", "--nonce", fixtureNonce, "--out-dir", filepath.Join(work, "auditor-bad"))
	aBadOut, err := aBad.CombinedOutput()
	if err == nil {
		t.Fatalf("run must fail when the built value differs from the TEE value:\n%s", aBadOut)
	}
	if !strings.Contains(string(aBadOut), "not registering") || !strings.Contains(string(aBadOut), "PCR0") {
		t.Fatalf("the reason for the check FAIL (PCR0) and 'not registering' must be printed:\n%s", aBadOut)
	}
	if strings.Contains(string(aBadOut), "registered") {
		t.Fatalf("registered although the check failed:\n%s", aBadOut)
	}

	// Auditor, check PASS: built value = TEE value, so register the built value and self-check.
	aRun := exec.Command(auditorBin, "audit",
		"--tee-addr", teeAddr, "--build-measurements", goodBuild, "--rpc", rpc, "--registry", auditorRegistryAddr, "--signing-key-file", auditorFile,
		"--service", "run-e2e", "--version", "1.0.0", "--nonce", fixtureNonce, "--out-dir", filepath.Join(work, "auditor"))
	aOut, err := aRun.CombinedOutput()
	if err != nil {
		t.Fatalf("auditor run: %v\n%s", err, aOut)
	}
	if !strings.Contains(string(aOut), "identical") || !strings.Contains(string(aOut), "PASS") {
		t.Fatalf("auditor run must print both check PASS and self-check PASS:\n%s", aOut)
	}
	aRecordKey := lastRecordKey(aOut)
	if aRecordKey == nil {
		t.Fatalf("auditor run: record key not found:\n%s", aOut)
	}

	// Vendor: requests RA from the same TEE itself and registers on its own contract. Its own contract has nothing
	// yet, so service/version are given directly. --arch is deliberately different from this machine's, so below we see whether User's verify
	// shows the architecture the chain recorded (the auditor side omits --arch, so it is this machine's GOARCH).
	vRun := exec.Command(vendorBin, "self-register",
		"--tee-addr", teeAddr, "--rpc", rpc, "--registry", vendorRegistryAddr, "--signing-key-file", vendorFile,
		"--service", "run-e2e", "--version", "1.0.0", "--arch", "arm64", "--nonce", fixtureNonce, "--out-dir", filepath.Join(work, "vendor"))
	vOut, err := vRun.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor run: %v\n%s", err, vOut)
	}
	if !strings.Contains(string(vOut), "PASS") {
		t.Fatalf("vendor run must print self-check PASS:\n%s", vOut)
	}
	vRecordKey := lastRecordKey(vOut)
	if vRecordKey == nil || string(vRecordKey) == string(aRecordKey) {
		t.Fatalf("vendor run registration record key equals the auditor's or was not captured:\n%s", vOut)
	}

	// Registering on the Auditor's contract with the Vendor's key is rejected: one registry = one approver.
	vWrong := exec.Command(vendorBin, "self-register",
		"--build-measurements", goodBuild, "--rpc", rpc, "--registry", auditorRegistryAddr, "--signing-key-file", vendorFile,
		"--service", "run-e2e", "--version", "1.0.0")
	if out, err := vWrong.CombinedOutput(); err == nil {
		t.Fatalf("the Vendor key is not the approver of the Auditor contract, so it must fail:\n%s", out)
	}

	// User: start to finish in one go (request, parse, lookup, compare), each contract looked at separately. The line of a matching
	// registration must show the architecture the chain recorded: User finds it without knowing the arch and reads it afterwards.
	for registryAddr, arch := range map[string]string{auditorRegistryAddr: runtime.GOARCH, vendorRegistryAddr: "arm64"} {
		uRun := exec.Command(userBin, "verify", "--tee-addr", teeAddr, "--rpc", rpc, "--registry", registryAddr,
			"--nonce", fixtureNonce, "--out-dir", t.TempDir())
		uOut, err := uRun.CombinedOutput()
		if err != nil {
			t.Fatalf("user run (%s): %v\n%s", registryAddr, err, uOut)
		}
		if !strings.Contains(string(uOut), "trusted —") {
			t.Fatalf("user run must say it is trusted on the registered contract:\n%s", uOut)
		}
		if want := "run-e2e 1.0.0 (" + arch + ") by"; !strings.Contains(string(uOut), want) {
			t.Fatalf("verify must name the architecture the chain recorded, lacks %q:\n%s", want, uOut)
		}
	}
}

// Running User run against a TEE whose measurements nobody registered: the lookup finds nothing, so exit 1.
func TestRun_E2E_User_NothingRegistered(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	rpc, stop := startAnvil(t)
	defer stop()

	keys := t.TempDir()
	deployerFile := writeKeyFile(t, keys, "deployer.key", deployerKey)

	bins := t.TempDir()
	auditorBin := buildBin(t, bins, "./cmd/auditor")
	vendorBin := buildBin(t, bins, "./cmd/vendor")
	userBin := buildBin(t, bins, "./cmd/user")

	registryAddr := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)

	fixture := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.cbor")
	fixtureNonce := filepath.Join(repoRootReg(t), "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.nonce")
	teeAddr, stopServe := startServe(t, vendorBin, fixture)
	defer stopServe()

	uRun := exec.Command(userBin, "verify", "--tee-addr", teeAddr, "--rpc", rpc, "--registry", registryAddr,
		"--nonce", fixtureNonce, "--out-dir", t.TempDir())
	out, err := uRun.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("exit 1 expected when nobody registered: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "nobody has registered") {
		t.Fatalf("the reason must be clear:\n%s", out)
	}
}
