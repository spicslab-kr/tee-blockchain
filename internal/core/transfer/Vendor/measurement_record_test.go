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

func mustErr(t *testing.T, r MeasurementRecord, want string) {
	t.Helper()
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: want error containing %q, got %v", r.Platform, want, err)
	}
}
