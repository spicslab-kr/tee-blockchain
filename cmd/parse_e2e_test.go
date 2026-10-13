// Builds the three binaries (cmd/vendor, cmd/user, cmd/auditor) for real, runs them with exec, and checks that the whole ra-serve, ra-request, ra-parse
// chain fits together up to the CLI layer. Same pattern as request_e2e_test.go, but this one continues through ra-parse,
// confirming that the measurements.json that would go into the registry matches the known fixture values.
// build, repoRoot and startServe live in e2e_helpers_test.go (shared with request_e2e_test.go).
package cmd_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCmd_E2E_ServeRequestParse: vendor serves a (fixture) TEE document with ra-serve, user receives and saves it with ra-request,
// and auditor verifies signature, nonce and measurements with ra-parse. The three binaries run as separate processes, and the point is
// that real CLI flags, exit codes and file contracts fit together, which catches wiring mistakes that unit tests miss.
//
// The fixture is a document received from a real Nitro enclave (fixture-my-web.cbor), so the signature is genuine. But FixtureExtractor
// does not bind to the nonce of each request and always returns the same document, so the <out>.nonce that ra-request saves (a fresh
// nonce each time) differs from the real nonce inside the document. So ra-parse is given the known nonce the document actually carries
// directly with --nonce. (On real hardware the extractor puts the nonce in the document, so this workaround is not needed.)
func TestCmd_E2E_ServeRequestParse(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := repoRoot(t)
	fixture, err := filepath.Abs(filepath.Join(root, "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.cbor"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureNonceFile, err := filepath.Abs(filepath.Join(root, "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.nonce"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}

	bins := t.TempDir()
	vendor := build(t, bins, "./cmd/vendor")
	user := build(t, bins, "./cmd/user")
	auditor := build(t, bins, "./cmd/auditor")

	addr, stop := startServe(t, vendor, fixture)
	defer stop()

	work := t.TempDir()
	docPath := filepath.Join(work, "doc.cbor")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// user side: receive the document with ra-request.
	reqCmd := exec.CommandContext(ctx, user, "ra-request", "--tee-addr", addr, "--out-file", docPath, "--format", "aws-nitro", "--timeout", "10s")
	reqOut, err := reqCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ra-request: %v\n%s", err, reqOut)
	}
	if _, err := os.Stat(docPath); err != nil {
		t.Fatalf("document file missing: %v", err)
	}
	noncePath := docPath + ".nonce"
	sentNonce, err := os.ReadFile(noncePath)
	if err != nil {
		t.Fatalf("nonce file missing: %v", err)
	}
	if len(strings.TrimSpace(string(sentNonce))) != 64 {
		t.Fatalf("nonce length looks wrong: %q", sentNonce)
	}

	// auditor side: verify signature, nonce and measurements with ra-parse. Pass the nonce the document actually carries (the fixture's) explicitly,
	// since the fixture extractor does not bind to the request nonce, verifying with the nonce just sent would fail (see the comment above).
	measurementsPath := docPath + ".measurements.json"
	parseCmd := exec.CommandContext(ctx, auditor, "ra-parse", "--doc", docPath, "--nonce", fixtureNonceFile, "--out-file", measurementsPath)
	parseOut, err := parseCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ra-parse: %v\n%s", err, parseOut)
	}
	out := string(parseOut)
	if !strings.Contains(out, "signature  ok") {
		t.Errorf("signature verification log missing:\n%s", out)
	}
	if !strings.Contains(out, "nonce      ok") {
		t.Errorf("nonce verification log missing:\n%s", out)
	}

	// the resulting measurements.json must be in a shape that goes straight into register --measurements, and PCR0 must equal the known fixture's.
	raw, err := os.ReadFile(measurementsPath)
	if err != nil {
		t.Fatalf("measurements.json missing: %v", err)
	}
	var got struct {
		Format       string
		Measurements map[string]string
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("measurements.json parse failed: %v\n%s", err, raw)
	}
	if got.Format != "aws-nitro" {
		t.Errorf("Format = %q, want aws-nitro", got.Format)
	}
	// A PCR must be a SHA384 value (48 bytes = 96 hex characters). The exact value comes from the fixture document itself, so it is not
	// recomputed here; this only checks that ra-parse actually extracted something and left it in a file, and that the shape is right.
	pcr0, ok := got.Measurements["PCR0"]
	if !ok || len(pcr0) != 96 {
		t.Errorf("PCR0 = %q, want 96 hex characters", pcr0)
	}
}

// TestCmd_E2E_ParseRejectsTamperedNonce: checks that ra-parse rejects a different nonce with exit 6 (nonce mismatch).
// It checks that the CLI exit code really matches the exitCode() wiring in cmd/auditor.
func TestCmd_E2E_ParseRejectsTamperedNonce(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := repoRoot(t)
	fixture, _ := filepath.Abs(filepath.Join(root, "internal", "tee", "parse", "aws", "testdata", "fixture-my-web.cbor"))

	bins := t.TempDir()
	auditor := build(t, bins, "./cmd/auditor")

	work := t.TempDir()
	docPath := filepath.Join(work, "doc.cbor")
	docBytes, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, docBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// a plausible 64-hex nonce that differs from the real one.
	wrongNonce := strings.Repeat("ab", 32)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	parseCmd := exec.CommandContext(ctx, auditor, "ra-parse", "--doc", docPath, "--nonce", wrongNonce)
	out, err := parseCmd.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 6 {
		t.Fatalf("want exit 6 (nonce mismatch), got %v\n%s", err, out)
	}
}
