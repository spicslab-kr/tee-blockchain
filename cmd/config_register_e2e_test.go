// build-config.json 을 등록과 전달 기록의 출처로 — Vendor 가 이미 build-config.json 에 service · version · build_env 를 적어
// 넘겼으니, Auditor 는 그걸 다시 타이핑하지 않는다. register/audit/self-register 는 --config 로 그 파일을 읽고, send 는
// --config + --measurements 로 measurement-record.json 을 직접 만든다. register 는 anvil 이 필요하고(없으면 건너뛴다),
// send 는 로컬 bare 저장소로 체인 없이 끝까지 돈다.
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

// writeBuildConfig — Vendor 가 send 로 넘기는 build-config.json (tb.build-config/1). arch 는 이 기계와 다르게 줄 수 있다 —
// 그래야 값이 플래그 기본값이 아니라 파일에서 왔다는 게 보인다.
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

// writeNitroMeasurements — nitro-cli 가 쓰는 out/measurements.json ({"Measurements":{"PCR0":hex,…}}).
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

// TestConfig_E2E_RegisterFromBuildConfig — register --config 만으로(--service/--version/--arch 없이) 등록되고, 등록 문장과
// build_env 힌트가 파일의 값을 말한다. 플래그를 같이 주면 플래그가 이기되 경고 한 줄. Vendor 쪽 register 도 같다.
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

	// 1. --config 만 — service · version · arch · go 전부 파일에서. 등록 문장, 확인 블록, build_env 힌트 셋 다.
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

	// 2. --config 와 --service other — 플래그가 이기고, 파일과 다르다는 경고 한 줄. version · arch 는 여전히 파일에서.
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

	// 3. 둘 다 없음 — 필수 오류가 --config 도 말한다. 체인에는 아무것도 안 간다.
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

	// 4. Vendor 의 register 도 --config 를 같은 방식으로 받는다 (self-register 가 그대로 넘긴다).
	out, err = exec.Command(vendorBin, "register", "--rpc", rpc, "--contract", vContract, "--key-file", vendorFile,
		"--measurements", measurements("d4"), "--config", cfg).CombinedOutput()
	if err != nil {
		t.Fatalf("vendor register --config: %v\n%s", err, out)
	}
	if want := "registered cfg-svc 2.0.0 (" + arch + ", " + goVer + " -trimpath)"; !bytes.Contains(out, []byte(want)) {
		t.Errorf("vendor register --config: output lacks %q:\n%s", want, out)
	}
}

// TestConfig_E2E_SendBuildsRecordFromConfig — send --config --measurements --image (--record 없이): 기록을 여기서 만들어
// 로컬 bare 저장소에 push 하고, measurements.json 옆에 measurement-record.json 도 남긴다. push 된 기록의 service · version ·
// build_env 는 config 그대로, values 는 measurements.json 의 PCR 그대로, build_tool.version 은 플래그 값.
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

	// --config 없이 --measurements 만, 또는 --image 없이 — 무엇이 빠졌는지 말하고 아무것도 보내지 않는다.
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
