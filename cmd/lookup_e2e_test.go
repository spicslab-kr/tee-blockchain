// lookup, through the real CLI: deploy and register for real on anvil (a local EVM), then query with the tb-auditor and tb-user lookup binaries.
// It reuses the deploy and anvil helpers of cmd/register_e2e_test.go (findAnvil, buildBin, writeKeyFile, ...) as they
// are: same package (cmd_test), so they are not rebuilt.
package cmd_test

import (
	"encoding/hex"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestLookup_E2E_FindAddressAndRead(t *testing.T) {
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

	// one signer per contract: the Auditor's and the Vendor's are deployed separately.
	deploy := exec.Command(auditorBin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--approver", auditorAddr)
	out, err := deploy.CombinedOutput()
	if err != nil {
		t.Fatalf("registry-deploy: %v\n%s", err, out)
	}
	contract := string(deployedAtRe.FindSubmatch(out)[1])
	vDeploy := exec.Command(vendorBin, "registry-deploy", "--rpc", rpc, "--key-file", deployerFile, "--approver", vendorAddr)
	vOut, err := vDeploy.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor registry-deploy: %v\n%s", err, vOut)
	}
	vContract := string(deployedAtRe.FindSubmatch(vOut)[1])

	measurements := "PCR0=" + strings.Repeat("aa", 48) + ",PCR1=" + strings.Repeat("bb", 48) + ",PCR2=" + strings.Repeat("cc", 48)
	reg := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements, "--service", "lookup-e2e", "--version", "2.0.0", "--platform", "aws-nitro", "--arch", "arm64")
	regOut, err := reg.CombinedOutput()
	if err != nil {
		t.Fatalf("register: %v\n%s", err, regOut)
	}
	key := string(keyLineRe.FindSubmatch(regOut)[1])

	// The Vendor registers the same measurements on its own contract: the contracts differ, so each is queried separately. The architecture is deliberately different,
	// so the lookup output shows whether the chain says which architecture each registration is for (it is not in the key, so keys still differ only by signer).
	vReg := exec.Command(vendorBin, "register", "--rpc", rpc, "--contract", vContract, "--key-file", vendorFile,
		"--measurements", measurements, "--service", "lookup-e2e", "--version", "2.0.0", "--platform", "aws-nitro", "--arch", "amd64")
	vRegOut, err := vReg.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor register: %v\n%s", err, vRegOut)
	}
	vendorKeyHex := string(keyLineRe.FindSubmatch(vRegOut)[1])
	if vendorKeyHex == key {
		t.Fatal("auditor and vendor keys must not be equal (the signers differ)")
	}

	// find by measurements: on the auditor contract. One signer, so one key, and it must be the auditor's.
	byMeasurements := exec.Command(auditorBin, "lookup", "--rpc", rpc, "--contract", contract, "--measurements", measurements)
	lmOut, err := byMeasurements.CombinedOutput()
	if err != nil {
		t.Fatalf("lookup --measurements: %v\n%s", err, lmOut)
	}
	if !strings.Contains(string(lmOut), "found 1 key(s)") || !strings.Contains(string(lmOut), key) {
		t.Fatalf("the auditor contract must yield exactly the auditor's key:\n%s", lmOut)
	}
	if strings.Contains(string(lmOut), vendorKeyHex) {
		t.Fatalf("the vendor's key got mixed into the auditor contract lookup:\n%s", lmOut)
	}
	if !strings.Contains(string(lmOut), "lookup-e2e 2.0.0 (aws-nitro, arm64)") {
		t.Errorf("service metadata (including architecture) not shown:\n%s", lmOut)
	}
	// finding the same measurements on the vendor contract gives exactly the vendor's key.
	vByMeasurements := exec.Command(vendorBin, "lookup", "--rpc", rpc, "--contract", vContract, "--measurements", measurements)
	vlmOut, err := vByMeasurements.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor lookup --measurements: %v\n%s", err, vlmOut)
	}
	if !strings.Contains(string(vlmOut), "found 1 key(s)") || !strings.Contains(string(vlmOut), vendorKeyHex) {
		t.Fatalf("the vendor contract must yield exactly the vendor's key:\n%s", vlmOut)
	}
	if !strings.Contains(string(vlmOut), "lookup-e2e 2.0.0 (aws-nitro, amd64)") {
		t.Errorf("the vendor registered as amd64, so the lookup must say so:\n%s", vlmOut)
	}

	// look up directly by the vendor-side key: the signer must be the vendor's address.
	vByKey := exec.Command(vendorBin, "lookup", "--rpc", rpc, "--contract", vContract, "--key", vendorKeyHex)
	vbkOut, err := vByKey.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor lookup --key: %v\n%s", err, vbkOut)
	}
	if !strings.Contains(string(vbkOut), vendorAddr) {
		t.Errorf("the signer of the vendor registration is not the vendor address:\n%s", vbkOut)
	}

	// read directly by key, as user (the side without registration rights can look up too).
	byKey := exec.Command(userBin, "lookup", "--rpc", rpc, "--contract", contract, "--key", key)
	bkOut, err := byKey.CombinedOutput()
	if err != nil {
		t.Fatalf("user lookup --key: %v\n%s", err, bkOut)
	}
	if !strings.Contains(string(bkOut), auditorAddr) {
		t.Errorf("the signer is not the auditor address:\n%s", bkOut)
	}
	if !strings.Contains(string(bkOut), "lookup-e2e 2.0.0 (aws-nitro, arm64)") {
		t.Errorf("the user lookup must show the registration's architecture too:\n%s", bkOut)
	}
	hashRe := regexp.MustCompile(`measurement hash\s+([0-9a-f]{64})`)
	m := hashRe.FindSubmatch(bkOut)
	if m == nil {
		t.Fatalf("measurement hash not found:\n%s", bkOut)
	}
	if _, err := hex.DecodeString(string(m[1])); err != nil {
		t.Errorf("measurement hash is not hex: %s", m[1])
	}

	// measurements that are not registered: an empty result.
	freshMeasurements := "PCR0=" + strings.Repeat("11", 48) + ",PCR1=" + strings.Repeat("22", 48) + ",PCR2=" + strings.Repeat("33", 48)
	notFound := exec.Command(userBin, "lookup", "--rpc", rpc, "--contract", contract, "--measurements", freshMeasurements)
	nfOut, err := notFound.CombinedOutput()
	if err != nil {
		t.Fatalf("lookup (unregistered measurements): %v\n%s", err, nfOut)
	}
	if !strings.Contains(string(nfOut), "nothing registered") {
		t.Errorf("found something for unregistered measurements:\n%s", nfOut)
	}

	// a key that is not registered: an error.
	badKey := exec.Command(userBin, "lookup", "--rpc", rpc, "--contract", contract, "--key", "0x"+strings.Repeat("00", 32))
	if out, err := badKey.CombinedOutput(); err == nil {
		t.Fatalf("lookup of an unregistered key must not succeed:\n%s", out)
	}
}
