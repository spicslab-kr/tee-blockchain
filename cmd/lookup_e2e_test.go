// lookup, through the real CLI: deploy and register for real on anvil (a local EVM), then query with the att-auditor and att-user lookup binaries.
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

	// one approver per registry: the Auditor's and the Vendor's are deployed separately.
	deploy := exec.Command(auditorBin, "registry-deploy", "--rpc", rpc, "--deployer-key-file", deployerFile, "--approver", auditorAddr)
	out, err := deploy.CombinedOutput()
	if err != nil {
		t.Fatalf("registry-deploy: %v\n%s", err, out)
	}
	registryAddr := string(deployedAtRe.FindSubmatch(out)[1])
	vDeploy := exec.Command(vendorBin, "registry-deploy", "--rpc", rpc, "--deployer-key-file", deployerFile, "--approver", vendorAddr)
	vOut, err := vDeploy.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor registry-deploy: %v\n%s", err, vOut)
	}
	vendorRegistryAddr := string(deployedAtRe.FindSubmatch(vOut)[1])

	measurements := "PCR0=" + strings.Repeat("aa", 48) + ",PCR1=" + strings.Repeat("bb", 48) + ",PCR2=" + strings.Repeat("cc", 48)
	reg := exec.Command(auditorBin, "register", "--rpc", rpc, "--registry", registryAddr, "--signing-key-file", auditorFile,
		"--measurements", measurements, "--service", "lookup-e2e", "--version", "2.0.0", "--platform", "aws-nitro", "--arch", "arm64")
	regOut, err := reg.CombinedOutput()
	if err != nil {
		t.Fatalf("register: %v\n%s", err, regOut)
	}
	recordKey := string(recordKeyLineRe.FindSubmatch(regOut)[1])

	// The Vendor registers the same measurements on its own contract: the contracts differ, so each is queried separately. The architecture is deliberately different,
	// so the lookup output shows whether the chain says which architecture each registration is for (the arch is display only, so record keys still differ only by approver).
	vReg := exec.Command(vendorBin, "register", "--rpc", rpc, "--registry", vendorRegistryAddr, "--signing-key-file", vendorFile,
		"--measurements", measurements, "--service", "lookup-e2e", "--version", "2.0.0", "--platform", "aws-nitro", "--arch", "amd64")
	vRegOut, err := vReg.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor register: %v\n%s", err, vRegOut)
	}
	vendorRecordKey := string(recordKeyLineRe.FindSubmatch(vRegOut)[1])
	if vendorRecordKey == recordKey {
		t.Fatal("auditor and vendor record keys must not be equal (the approvers differ)")
	}

	// find by measurements: on the auditor contract. One approver, so one record key, and it must be the auditor's.
	byMeasurements := exec.Command(auditorBin, "lookup", "--rpc", rpc, "--registry", registryAddr, "--measurements", measurements)
	lmOut, err := byMeasurements.CombinedOutput()
	if err != nil {
		t.Fatalf("lookup --measurements: %v\n%s", err, lmOut)
	}
	if !strings.Contains(string(lmOut), "found 1 record key(s)") || !strings.Contains(string(lmOut), recordKey) {
		t.Fatalf("the auditor registry must yield exactly the auditor's record key:\n%s", lmOut)
	}
	if strings.Contains(string(lmOut), vendorRecordKey) {
		t.Fatalf("the vendor's record key got mixed into the auditor registry lookup:\n%s", lmOut)
	}
	if !strings.Contains(string(lmOut), "lookup-e2e 2.0.0 (aws-nitro, arm64)") {
		t.Errorf("service metadata (including architecture) not shown:\n%s", lmOut)
	}
	// finding the same measurements on the vendor contract gives exactly the vendor's key.
	vByMeasurements := exec.Command(vendorBin, "lookup", "--rpc", rpc, "--registry", vendorRegistryAddr, "--measurements", measurements)
	vlmOut, err := vByMeasurements.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor lookup --measurements: %v\n%s", err, vlmOut)
	}
	if !strings.Contains(string(vlmOut), "found 1 record key(s)") || !strings.Contains(string(vlmOut), vendorRecordKey) {
		t.Fatalf("the vendor registry must yield exactly the vendor's record key:\n%s", vlmOut)
	}
	if !strings.Contains(string(vlmOut), "lookup-e2e 2.0.0 (aws-nitro, amd64)") {
		t.Errorf("the vendor registered as amd64, so the lookup must say so:\n%s", vlmOut)
	}

	// look up directly by the vendor-side record key: the approver must be the vendor's address.
	vByRecordKey := exec.Command(vendorBin, "lookup", "--rpc", rpc, "--registry", vendorRegistryAddr, "--record-key", vendorRecordKey)
	vbkOut, err := vByRecordKey.CombinedOutput()
	if err != nil {
		t.Fatalf("vendor lookup --record-key: %v\n%s", err, vbkOut)
	}
	if !strings.Contains(string(vbkOut), vendorAddr) {
		t.Errorf("the approver of the vendor registration is not the vendor address:\n%s", vbkOut)
	}

	// read directly by record key, as user (the side without registration rights can look up too).
	byRecordKey := exec.Command(userBin, "lookup", "--rpc", rpc, "--registry", registryAddr, "--record-key", recordKey)
	bkOut, err := byRecordKey.CombinedOutput()
	if err != nil {
		t.Fatalf("user lookup --record-key: %v\n%s", err, bkOut)
	}
	if !strings.Contains(string(bkOut), auditorAddr) {
		t.Errorf("the approver is not the auditor address:\n%s", bkOut)
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
	notFound := exec.Command(userBin, "lookup", "--rpc", rpc, "--registry", registryAddr, "--measurements", freshMeasurements)
	nfOut, err := notFound.CombinedOutput()
	if err != nil {
		t.Fatalf("lookup (unregistered measurements): %v\n%s", err, nfOut)
	}
	if !strings.Contains(string(nfOut), "nothing registered") {
		t.Errorf("found something for unregistered measurements:\n%s", nfOut)
	}

	// a key that is not registered: an error.
	badRecordKey := exec.Command(userBin, "lookup", "--rpc", rpc, "--registry", registryAddr, "--record-key", "0x"+strings.Repeat("00", 32))
	if out, err := badRecordKey.CombinedOutput(); err == nil {
		t.Fatalf("lookup of an unregistered record key must not succeed:\n%s", out)
	}
}
