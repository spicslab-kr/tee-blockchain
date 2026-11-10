package vendor

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMeasurementRecord_PerPlatform(t *testing.T) {
	for _, r := range []MeasurementRecord{recAWS(), recGCP(), recAzure()} {
		if err := r.Validate(); err != nil {
			t.Errorf("%s: complete record must validate: %v", r.Platform, err)
		}
	}
	// aws-nitro: EIF file, PCR0/1/2, tool and version
	r := recAWS()
	r.Image.Path = ""
	mustErr(t, r, "image.path")
	r = recAWS()
	r.Image.SHA256 = "zz"
	mustErr(t, r, "image.sha256")
	r = recAWS()
	r.Values = r.Values[:2] // PCR2 missing
	mustErr(t, r, "missing PCR2")
	r = recAWS()
	r.BuildTool.Version = ""
	mustErr(t, r, "build_tool.version")
	// gcp: digest (not a tag), tool
	r = recGCP()
	r.Image.Digest = "gcr.io/proj/svc:latest"
	mustErr(t, r, "image.digest")
	r = recGCP()
	r.Values = nil
	mustErr(t, r, "values")
	r = recGCP()
	r.BuildTool.Name = ""
	mustErr(t, r, "build_tool.name")
	// azure: digest or image_id
	r = recAzure()
	r.Image.ImageID = ""
	mustErr(t, r, "digest (container) or image_id")
	r = recAzure()
	r.Image = Image{Digest: "sha256:" + strings.Repeat("ef", 32)}
	if err := r.Validate(); err != nil {
		t.Errorf("azure container digest must validate: %v", err)
	}
	// common
	r = recAWS()
	r.Values = append(r.Values, struct {
		Name  string `json:"name"`
		Value []byte `json:"value"`
	}{Name: "", Value: nil})
	mustErr(t, r, "values[3]")
}

// The registration (chain and registry address) is optional. When present, both fields must be filled, and it is encoded only then.
func TestMeasurementRecord_Registration(t *testing.T) {
	r := recAWS()
	r.Registration = &Registration{Chain: "ether", Address: "0xabc"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	b, _ := Encode(r)
	if !strings.Contains(string(b), `"registration"`) {
		t.Fatal("registration must be encoded when present")
	}
	got, err := DecodeMeasurementRecord(b)
	if err != nil || got.Registration == nil || got.Registration.Address != "0xabc" {
		t.Fatalf("registration lost: %v %+v", err, got.Registration)
	}
	r.Registration = &Registration{Chain: "ether"}
	mustErr(t, r, "registration")
	b, _ = Encode(recAWS())
	if strings.Contains(string(b), `"registration"`) {
		t.Fatal("registration must be omitted when absent")
	}
}

// The same source gives different PCRs on x86_64 and arm64, and under a different Go toolchain. An aws-nitro
// record therefore names its build environment (build_env: os, arch, go, trimpath), in the shape of build_env
// in build-config.json. The arch spelling is GOARCH (amd64, arm64) from build.ArchTargets; x86_64 and aarch64
// are rejected.
func TestMeasurementRecord_BuildEnvArch(t *testing.T) {
	for _, ok := range []string{"amd64", "arm64"} {
		r := recAWS()
		r.BuildEnv.Arch = ok
		if err := r.Validate(); err != nil {
			t.Errorf("build_env.arch %q must validate: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "x86_64", "aarch64", "AMD64", "arm"} {
		r := recAWS()
		r.BuildEnv.Arch = bad
		mustErr(t, r, "build_env.arch: want one of amd64|arm64")
	}
	// gcp and azure do not require build_env: their measurement is an image digest.
	for _, r := range []MeasurementRecord{recGCP(), recAzure()} {
		if err := r.Validate(); err != nil {
			t.Errorf("%s without build_env must still validate: %v", r.Platform, err)
		}
	}
	// When present, build_env.arch must be an architecture this project builds, on any platform.
	r := recGCP()
	r.BuildEnv.Arch = "mips"
	mustErr(t, r, "build_env.arch: unknown")
}

// An aws-nitro record requires the OS and Go toolchain too; the Vendor reads them to learn which toolchain built the image.
func TestMeasurementRecord_BuildEnvRequiresOSAndGoOnAWS(t *testing.T) {
	r := recAWS()
	r.BuildEnv.OS = ""
	mustErr(t, r, "build_env.os: required for aws-nitro")
	r = recAWS()
	r.BuildEnv.Go = ""
	mustErr(t, r, "build_env.go: required for aws-nitro")
	// trimpath=false is allowed; it records how the image was built.
	r = recAWS()
	r.BuildEnv.Trimpath = false
	if err := r.Validate(); err != nil {
		t.Errorf("trimpath=false must validate: %v", err)
	}
	// A missing build_env reports all three required fields.
	r = recAWS()
	r.BuildEnv = BuildEnv{}
	for _, want := range []string{"build_env.os", "build_env.arch", "build_env.go"} {
		mustErr(t, r, want)
	}
}

// build_env round-trips through the file unchanged, so the receiving Vendor can tell which architecture and toolchain the record is for.
func TestMeasurementRecord_BuildEnvRoundTrip(t *testing.T) {
	r := recAWS()
	r.BuildEnv = BuildEnv{OS: "linux", Arch: "arm64", Go: "go1.26.1", Trimpath: true}
	b, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"build_env"`) || !strings.Contains(string(b), `"arch": "arm64"`) || !strings.Contains(string(b), `"go": "go1.26.1"`) {
		t.Fatalf("build_env must be encoded:\n%s", b)
	}
	got, err := DecodeMeasurementRecord(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.BuildEnv != r.BuildEnv {
		t.Fatalf("build_env lost in round trip: %+v", got.BuildEnv)
	}
	// The receiver rejects a file without build_env, and the message names build_env.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "build_env")
	stripped, _ := json.Marshal(m)
	if _, err := DecodeMeasurementRecord(stripped); err == nil || !strings.Contains(err.Error(), "build_env") {
		t.Fatalf("record without build_env must be rejected naming build_env, got %v", err)
	}
}

func mustErr(t *testing.T, r MeasurementRecord, want string) {
	t.Helper()
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: want error containing %q, got %v", r.Platform, want, err)
	}
}
