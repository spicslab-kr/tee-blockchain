package auditor

import (
	"strings"
	"testing"
)

// Format regression: if the JSON changes it no longer matches this and the test fails. If the change is intended, update the expected value.
const (
	wantBuildConfigJSON = `{
  "schema": "tb.build-config/1",
  "service": "svc",
  "version": "1.2.0",
  "platform": "aws",
  "source": {
    "repo": "https://example.com/svc.git",
    "commit": "0123456789abcdef0123456789abcdef01234567"
  },
  "tee_info": {
    "ami_id": "ami-0123",
    "instance_id": "i-0abc",
    "region": "ap-northeast-2"
  },
  "build_env": {
    "os": "linux",
    "arch": "amd64",
    "go": "go1.26.1",
    "trimpath": true
  }
}
`
	wantMeasurementRecordJSON = `{
  "schema": "tb.measurement-record/1",
  "service": "svc",
  "version": "1.2.0",
  "platform": "aws",
  "build_env": {
    "os": "linux",
    "arch": "amd64",
    "go": "go1.26.1",
    "trimpath": true
  },
  "image": {
    "path": "svc.eif",
    "sha256": "abababababababababababababababababababababababababababababababab"
  },
  "values": [
    {
      "name": "PCR0",
      "value": "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEB"
    },
    {
      "name": "PCR1",
      "value": "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIC"
    },
    {
      "name": "PCR2",
      "value": "AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMD"
    }
  ],
  "build_tool": {
    "name": "nitro-cli",
    "version": "1.3.0"
  }
}
`
)

func TestEncode_MatchesSpec(t *testing.T) {
	for name, tc := range map[string]struct {
		v    any
		want string
	}{"build_config": {cfgAWS(), wantBuildConfigJSON}, "measurement_record": {recAWS(), wantMeasurementRecordJSON}} {
		got, err := Encode(tc.v)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.want {
			t.Errorf("%s: encoding drifted from spec\n--- got\n%s\n--- want\n%s", name, got, tc.want)
		}
	}
}

func TestDecode_RoundTrip_AllPlatforms(t *testing.T) {
	for _, c := range []BuildConfig{cfgAWS(), cfgGCP(), cfgAzure()} {
		b, _ := Encode(c)
		got, err := DecodeBuildConfig(b)
		if err != nil {
			t.Fatalf("%s: %v", c.Platform, err)
		}
		if got.Platform != c.Platform || got.Source.Commit != commit || len(got.TEEInfo) != len(c.TEEInfo) {
			t.Errorf("%s: round trip changed the config", c.Platform)
		}
	}
	for _, r := range []MeasurementRecord{recAWS(), recGCP(), recAzure()} {
		b, _ := Encode(r)
		got, err := DecodeMeasurementRecord(b)
		if err != nil {
			t.Fatalf("%s: %v", r.Platform, err)
		}
		if len(got.Values) != len(r.Values) || got.Values[0].Name != r.Values[0].Name {
			t.Errorf("%s: measurements changed", r.Platform)
		}
	}
}

func TestDecode_UnknownFieldRejected(t *testing.T) {
	b, _ := Encode(cfgAWS())
	b = []byte(strings.Replace(string(b), `"schema"`, `"extra": 1, "schema"`, 1))
	if _, err := DecodeBuildConfig(b); err == nil {
		t.Fatal("build config: unknown field must be rejected")
	}
	b, _ = Encode(recAWS())
	b = []byte(strings.Replace(string(b), `"schema"`, `"extra": 1, "schema"`, 1))
	if _, err := DecodeMeasurementRecord(b); err == nil {
		t.Fatal("measurement record: unknown field must be rejected")
	}
}

func TestDecode_WrongSchemaRejected(t *testing.T) {
	b, _ := Encode(recAWS())
	b = []byte(strings.Replace(string(b), SchemaMeasurementRecord, SchemaBuildConfig, 1))
	if _, err := DecodeMeasurementRecord(b); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("want schema error, got %v", err)
	}
}

// A config missing the keys required for its platform is rejected.
