// The CVM flow scripts under e2e/cvm need two TDX or SEV-SNP guests and a chain, so they are never run here for real:
// e2e/cvm/check-flow.sh runs them against stand-in gcloud, go, curl and att commands and checks the calls each step
// makes, and runs the guest-side script with a stand-in sudo and att. This test runs that script.
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCVMFlowScripts(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	// go test caches a passing result by the files the test itself opens, not by what bash reads. Reading the scripts
	// here makes a change to any of them run this test again.
	for _, f := range []string{
		"e2e/env.sh", "e2e/cvm/check-flow.sh", "e2e/cvm/env-cvm.sh", "e2e/cvm/0-Setup-guests.sh",
		"e2e/cvm/guest/guest-step.sh",
		"e2e/cvm/flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh",
		"e2e/cvm/flow1-auditor-registers/2-Vendor-build-launch-check-record.sh",
		"e2e/cvm/flow1-auditor-registers/2b-Auditor-audit-vendor.sh",
		"e2e/cvm/flow1-auditor-registers/4-Vendor-tamper-rebuild-launch.sh",
		"e2e/cvm/flow1-auditor-registers/4b-Auditor-audit-vendor-tampered.sh",
		"e2e/cvm/flow2-vendor-self-registers/1-Vendor-build-launch-self-register.sh",
		"e2e/cvm/flow2-vendor-self-registers/3-Vendor-tamper-rebuild-self-register.sh",
		"e2e/cvm/flow2-vendor-self-registers/4-Auditor-audit-vendor.sh",
	} {
		if _, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(bash, "e2e/cvm/check-flow.sh")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("e2e/cvm/check-flow.sh: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
