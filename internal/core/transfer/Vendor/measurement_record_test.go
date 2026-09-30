package vendor

import (
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

// 같은 소스도 x86_64 와 arm64 에서 PCR 이 다르다. 그래서 aws 기록은 어느 아키텍처 것인지(arch) 반드시 적는다.
// 철자는 GOARCH(amd64 · arm64) 하나로 — uname 이 내는 x86_64 · aarch64 는 거절한다. 출처는 build.ArchTargets.
func TestMeasurementRecord_Arch(t *testing.T) {
	for _, ok := range []string{"amd64", "arm64"} {
		r := recAWS()
		r.Arch = ok
		if err := r.Validate(); err != nil {
			t.Errorf("arch %q must validate: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "x86_64", "aarch64", "AMD64", "arm"} {
		r := recAWS()
		r.Arch = bad
		mustErr(t, r, "arch: want one of amd64|arm64")
	}
	// gcp · azure 는 아직 arch 를 요구하지 않는다 (측정값이 이미지 digest 라 아키텍처가 따로 갈리지 않는다).
	for _, r := range []MeasurementRecord{recGCP(), recAzure()} {
		if err := r.Validate(); err != nil {
			t.Errorf("%s without arch must still validate: %v", r.Platform, err)
		}
	}
}

// arch 는 파일에 그대로 실려 왕복해야 한다 — 받는 쪽(Vendor)이 어느 아키텍처의 기록인지 알 수 있게.
func TestMeasurementRecord_ArchRoundTrip(t *testing.T) {
	r := recAWS()
	r.Arch = "arm64"
	b, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"arch": "arm64"`) && !strings.Contains(string(b), `"arch":"arm64"`) {
		t.Fatalf("arch must be encoded:\n%s", b)
	}
	got, err := DecodeMeasurementRecord(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Arch != "arm64" {
		t.Fatalf("arch lost in round trip: %q", got.Arch)
	}
	// arch 가 빠진 파일은 받는 쪽에서 거절한다.
	stripped := strings.Replace(string(b), `"arch": "arm64",`, "", 1)
	stripped = strings.Replace(stripped, `"arch":"arm64",`, "", 1)
	if _, err := DecodeMeasurementRecord([]byte(stripped)); err == nil || !strings.Contains(err.Error(), "arch") {
		t.Fatalf("record without arch must be rejected naming arch, got %v", err)
	}
}

func mustErr(t *testing.T, r MeasurementRecord, want string) {
	t.Helper()
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: want error containing %q, got %v", r.Platform, want, err)
	}
}
