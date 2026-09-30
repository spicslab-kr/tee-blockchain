package auditor

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
	// aws: EIF 파일 · PCR0·1·2 · 도구와 버전
	r := recAWS()
	r.Image.Path = ""
	mustErr(t, r, "image.path")
	r = recAWS()
	r.Image.SHA256 = "zz"
	mustErr(t, r, "image.sha256")
	r = recAWS()
	r.Values = r.Values[:2] // PCR2 빠짐
	mustErr(t, r, "missing PCR2")
	r = recAWS()
	r.BuildTool.Version = ""
	mustErr(t, r, "build_tool.version")
	// gcp: digest (태그 아님) · 도구
	r = recGCP()
	r.Image.Digest = "gcr.io/proj/svc:latest"
	mustErr(t, r, "image.digest")
	r = recGCP()
	r.Values = nil
	mustErr(t, r, "values")
	r = recGCP()
	r.BuildTool.Name = ""
	mustErr(t, r, "build_tool.name")
	// azure: digest 또는 image_id
	r = recAzure()
	r.Image.ImageID = ""
	mustErr(t, r, "digest (container) or image_id")
	r = recAzure()
	r.Image = Image{Digest: "sha256:" + strings.Repeat("ef", 32)}
	if err := r.Validate(); err != nil {
		t.Errorf("azure container digest must validate: %v", err)
	}
	// 공통
	r = recAWS()
	r.Values = append(r.Values, struct {
		Name  string `json:"name"`
		Value []byte `json:"value"`
	}{Name: "", Value: nil})
	mustErr(t, r, "values[3]")
}

// 시퀀스 9 — "이미지 파일 및 등록값 제공". 등록값은 선택이지만 있으면 둘 다 채워야 한다.
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

// 같은 소스도 x86_64 와 arm64 에서, 그리고 Go 툴체인이 다르면 PCR 이 다르다. 그래서 aws 기록은 어느 빌드 환경 것인지
// (build_env: os · arch · go · trimpath) 반드시 적는다 — build-config.json 의 build_env 와 같은 모양이다.
// arch 철자는 GOARCH(amd64 · arm64) 하나로 — uname 이 내는 x86_64 · aarch64 는 거절한다. 출처는 build.ArchTargets.
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
	// gcp · azure 는 아직 build_env 를 요구하지 않는다 (측정값이 이미지 digest 라 아키텍처가 따로 갈리지 않는다).
	for _, r := range []MeasurementRecord{recGCP(), recAzure()} {
		if err := r.Validate(); err != nil {
			t.Errorf("%s without build_env must still validate: %v", r.Platform, err)
		}
	}
	// 다만 적혀 있다면 이 프로젝트가 빌드하는 아키텍처여야 한다 — 어느 플랫폼이든.
	r := recGCP()
	r.BuildEnv.Arch = "mips"
	mustErr(t, r, "build_env.arch: unknown")
}

// aws 기록은 OS 와 Go 툴체인도 비워 둘 수 없다 — Vendor 가 이 기록으로 어느 툴체인이 이미지를 만들었는지 안다.
func TestMeasurementRecord_BuildEnvRequiresOSAndGoOnAWS(t *testing.T) {
	r := recAWS()
	r.BuildEnv.OS = ""
	mustErr(t, r, "build_env.os: required for aws")
	r = recAWS()
	r.BuildEnv.Go = ""
	mustErr(t, r, "build_env.go: required for aws")
	// trimpath=false 는 허용한다 — 사실을 적는 것이지 요구 조건이 아니다.
	r = recAWS()
	r.BuildEnv.Trimpath = false
	if err := r.Validate(); err != nil {
		t.Errorf("trimpath=false must validate: %v", err)
	}
	// 통째로 빠진 build_env 는 세 필드를 모두 지목한다.
	r = recAWS()
	r.BuildEnv = BuildEnv{}
	for _, want := range []string{"build_env.os", "build_env.arch", "build_env.go"} {
		mustErr(t, r, want)
	}
}

// build_env 는 파일에 그대로 실려 왕복해야 한다 — 받는 쪽(Vendor)이 어느 아키텍처·어느 툴체인의 기록인지 알 수 있게.
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
	// build_env 가 빠진 파일은 받는 쪽에서 거절한다 — 메시지가 build_env 를 지목한다.
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
