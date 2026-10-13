// build-config.json as the source for registration and the hand-over record. The Vendor has already handed service, version and build_env over in build-config.json,
// so the Auditor does not type them again. register/audit/self-register read that file with --config, and send builds
// measurement-record.json directly from --config + --measurements. register needs anvil (skipped without it),
// and send runs to the end against a local bare repository, no chain needed.
package cmd_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeBuildConfig writes the build-config.json that the Vendor hands over with send (tb.build-config/1). arch may differ from this machine's,
// so it is visible that the value came from the file and not from the flag default.
func writeBuildConfig(t *testing.T, dir, service, version, arch, goVer string) string {
	t.Helper()
	body := `{
  "schema": "tb.build-config/1",
  "service": "` + service + `",
  "version": "` + version + `",
  "platform": "aws",
  "source": { "repo": "https://example.com/svc.git", "commit": "0123456789abcdef0123456789abcdef01234567", "path": "mainline/alpine-slim" },
  "tee_info": { "instance_id": "i-0abc", "region": "eu-north-1", "ami_id": "ami-0123" },
  "build_env": { "os": "linux", "arch": "` + arch + `", "go": "` + goVer + `", "trimpath": true }
}
`
	p := filepath.Join(dir, "build-config.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeNitroMeasurements writes the out/measurements.json that nitro-cli produces ({"Measurements":{"PCR0":hex,...}}).
func writeNitroMeasurements(t *testing.T, dir string, fill byte) (path string, pcr map[string][]byte) {
	t.Helper()
	pcr = map[string][]byte{
		"PCR0": bytes.Repeat([]byte{fill}, 48),
		"PCR1": bytes.Repeat([]byte{fill + 1}, 48),
		"PCR2": bytes.Repeat([]byte{fill + 2}, 48),
	}
	body := `{ "Measurements": { "HashAlgorithm": "Sha384 { ... }", "PCR0": "` + hex.EncodeToString(pcr["PCR0"]) +
		`", "PCR1": "` + hex.EncodeToString(pcr["PCR1"]) + `", "PCR2": "` + hex.EncodeToString(pcr["PCR2"]) + `" } }`
	path = filepath.Join(dir, "measurements.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, pcr
}

// TestConfig_E2E_RegisterFromBuildConfig: register with --config alone (no --service/--version/--arch) registers, and the registration sentence and
// build_env hint state the file's values. If flags are also given, the flags win with a one-line warning. The Vendor's register behaves the same.
func TestConfig_E2E_RegisterFromBuildConfig(t *testing.T) {
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
	contract := deployFor(t, auditorBin, rpc, deployerFile, auditorAddr)
	vContract := deployFor(t, vendorBin, rpc, deployerFile, vendorAddr)

	goVer, arch := runtime.Version(), otherArch()
	cfg := writeBuildConfig(t, t.TempDir(), "cfg-svc", "2.0.0", arch, goVer)
	measurements := func(fill string) string {
		return "PCR0=" + strings.Repeat(fill, 48) + ",PCR1=" + strings.Repeat(fill, 48) + ",PCR2=" + strings.Repeat(fill, 48)
	}

	// 1. --config only: service, version, arch and go all from the file. All three of registration sentence, confirmation block and build_env hint.
	out, err := exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements("a1"), "--config", cfg).CombinedOutput()
	if err != nil {
		t.Fatalf("register --config: %v\n%s", err, out)
	}
	for _, want := range []string{
		"registering cfg-svc 2.0.0 (" + arch + ", " + goVer + " -trimpath)",
		"registered cfg-svc 2.0.0 (" + arch + ", " + goVer + " -trimpath)",
		"build env  linux/" + arch + " " + goVer + " trimpath=true",
		`"build_env": {"os":"linux","arch":"` + arch + `","go":"` + goVer + `","trimpath":true}`,
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("register --config: output lacks %q:\n%s", want, out)
		}
	}
	if bytes.Contains(out, []byte("warning:")) {
		t.Errorf("--config alone must not warn:\n%s", out)
	}
	if keyLineRe.FindSubmatch(out) == nil {
		t.Fatalf("no registration key in output:\n%s", out)
	}

	// 2. --config and --service other: the flag wins, with a one-line warning that it differs from the file. version and arch still come from the file.
	out, err = exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements("b2"), "--config", cfg, "--service", "other").CombinedOutput()
	if err != nil {
		t.Fatalf("register --config --service other: %v\n%s", err, out)
	}
	for _, want := range []string{
		"warning: --service other differs from build-config.json (cfg-svc)",
		"registered other 2.0.0 (" + arch + ", " + goVer + " -trimpath)",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("register --config --service other: output lacks %q:\n%s", want, out)
		}
	}
	if bytes.Contains(out, []byte("--version")) || bytes.Contains(out, []byte("--arch")) {
		t.Errorf("only --service was given, so only --service may be warned about:\n%s", out)
	}

	// 3. neither: the required-value error mentions --config too. Nothing goes to the chain.
	out, err = exec.Command(auditorBin, "register", "--rpc", rpc, "--contract", contract, "--key-file", auditorFile,
		"--measurements", measurements("c3")).CombinedOutput()
	if err == nil {
		t.Fatalf("register with neither --config nor --service/--version must fail:\n%s", out)
	}
	for _, want := range []string{"--service", "--version", "--config"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("the required-flags error must name %q:\n%s", want, out)
		}
	}
	if bytes.Contains(out, []byte("registered")) {
		t.Errorf("nothing must be registered without service/version:\n%s", out)
	}

	// 4. the Vendor's register takes --config the same way (self-register passes it straight through).
	out, err = exec.Command(vendorBin, "register", "--rpc", rpc, "--contract", vContract, "--key-file", vendorFile,
		"--measurements", measurements("d4"), "--config", cfg).CombinedOutput()
	if err != nil {
		t.Fatalf("vendor register --config: %v\n%s", err, out)
	}
	if want := "registered cfg-svc 2.0.0 (" + arch + ", " + goVer + " -trimpath)"; !bytes.Contains(out, []byte(want)) {
		t.Errorf("vendor register --config: output lacks %q:\n%s", want, out)
	}
}

// TestConfig_E2E_SendBuildsRecordFromConfig: send --config --measurements --image (without --record) builds the record here,
// pushes it to a local bare repository, and also leaves a measurement-record.json next to measurements.json. In the pushed record service, version and
// build_env are the config's as they are, values are the PCRs of measurements.json as they are, and build_tool.version is the flag's value.
func TestConfig_E2E_SendBuildsRecordFromConfig(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	auditorBin := buildBin(t, t.TempDir(), "./cmd/auditor")
	goVer, arch := runtime.Version(), otherArch()
	work := t.TempDir()
	cfg := writeBuildConfig(t, work, "cfg-svc", "2.0.0", arch, goVer)
	outDir := filepath.Join(work, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meas, pcr := writeNitroMeasurements(t, outDir, 0x11)
	eif := filepath.Join(outDir, "svc.eif")
	if err := os.WriteFile(eif, []byte("eif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := gitBare(t)
	cmd := exec.Command(auditorBin, "send", "--repo", remote, "--config", cfg, "--measurements", meas, "--image", eif, "--build-tool-version", "1.5.0")
	gitIdentity(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("send --config --measurements: %v\n%s", err, out)
	}
	for _, want := range []string{
		"record cfg-svc 2.0.0 aws · build env: linux/" + arch + " " + goVer + " trimpath=true",
		"record written to " + filepath.Join(outDir, "measurement-record.json"),
		"sent ",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("send --config: output lacks %q:\n%s", want, out)
		}
	}

	type record struct {
		Schema, Service, Version, Platform string
		BuildEnv                           struct {
			OS, Arch, Go string
			Trimpath     bool
		} `json:"build_env"`
		Image  struct{ Path, SHA256 string }
		Values []struct {
			Name  string
			Value []byte
		}
		BuildTool struct{ Name, Version string } `json:"build_tool"`
	}
	check := func(where string, raw []byte) {
		var rec record
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("%s is not JSON: %v\n%s", where, err, raw)
		}
		if rec.Schema != "tb.measurement-record/1" || rec.Service != "cfg-svc" || rec.Version != "2.0.0" || rec.Platform != "aws" {
			t.Errorf("%s: header drifted from the config: %+v", where, rec)
		}
		if rec.BuildEnv.OS != "linux" || rec.BuildEnv.Arch != arch || rec.BuildEnv.Go != goVer || !rec.BuildEnv.Trimpath {
			t.Errorf("%s: build_env drifted from the config: %+v", where, rec.BuildEnv)
		}
		if rec.Image.Path != "svc.eif" || len(rec.Image.SHA256) != 64 {
			t.Errorf("%s: image must be the EIF's file name with its sha256: %+v", where, rec.Image)
		}
		if rec.BuildTool.Name != "nitro-cli" || rec.BuildTool.Version != "1.5.0" {
			t.Errorf("%s: build_tool must be nitro-cli 1.5.0: %+v", where, rec.BuildTool)
		}
		got := map[string][]byte{}
		for _, v := range rec.Values {
			got[v.Name] = v.Value
		}
		for name, want := range pcr {
			if !bytes.Equal(got[name], want) {
				t.Errorf("%s: %s = %s, want %s (from measurements.json)", where, name, base64.StdEncoding.EncodeToString(got[name]), base64.StdEncoding.EncodeToString(want))
			}
		}
	}
	pushed, err := exec.Command("git", "-C", remote, "show", "HEAD:measurement-record.json").CombinedOutput()
	if err != nil {
		t.Fatalf("read pushed record: %v\n%s", err, pushed)
	}
	check("pushed measurement-record.json", pushed)
	beside, err := os.ReadFile(filepath.Join(outDir, "measurement-record.json"))
	if err != nil {
		t.Fatalf("send must leave measurement-record.json next to measurements.json: %v", err)
	}
	check("measurement-record.json beside measurements.json", beside)

	// --measurements without --config, or without --image: say what is missing and send nothing.
	cmd = exec.Command(auditorBin, "send", "--repo", gitBare(t), "--measurements", meas, "--image", eif)
	gitIdentity(cmd)
	if out, err := cmd.CombinedOutput(); err == nil || !bytes.Contains(out, []byte("--config")) || bytes.Contains(out, []byte("sent ")) {
		t.Errorf("send without --record or --config must refuse and name --config: %v\n%s", err, out)
	}
	cmd = exec.Command(auditorBin, "send", "--repo", gitBare(t), "--config", cfg, "--measurements", meas, "--build-tool-version", "1.5.0")
	gitIdentity(cmd)
	if out, err := cmd.CombinedOutput(); err == nil || !bytes.Contains(out, []byte("--image")) || bytes.Contains(out, []byte("sent ")) {
		t.Errorf("send --config without --image must refuse and name --image: %v\n%s", err, out)
	}
}
