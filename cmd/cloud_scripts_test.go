// The cloud scripts under e2e/cloud make billable instances, so they are never run here for real: e2e/cloud/check-scripts.sh
// runs every action of each against stand-in aws, gcloud and curl commands and checks the calls each action makes.
// This test runs that script, so that go test ./... catches an action that breaks before its CLI call, as stop and
// start did in both AWS scripts until 28e1e1e.
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCloudScripts(t *testing.T) {
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
	for _, f := range []string{"check-scripts.sh", "aws-nitro.sh", "aws-snp.sh", "gcp-cvm.sh"} {
		if _, err := os.ReadFile(filepath.Join(root, "e2e", "cloud", f)); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(bash, "e2e/cloud/check-scripts.sh")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("e2e/cloud/check-scripts.sh: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
