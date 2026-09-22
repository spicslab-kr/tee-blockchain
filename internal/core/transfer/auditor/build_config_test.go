package auditor

import (
	"strings"
	"testing"
)

func TestBuildConfig_TEEInfoKeysPerPlatform(t *testing.T) {
	cases := map[string]struct {
		cfg  BuildConfig
		drop string
	}{
		"aws needs ami_id":          {cfgAWS(), "ami_id"},
		"aws needs region":          {cfgAWS(), "region"},
		"aws needs instance_id":     {cfgAWS(), "instance_id"},
		"gcp needs container_image": {cfgGCP(), "container_image"},
		"gcp needs project_id":      {cfgGCP(), "project_id"},
		"azure needs maa_endpoint":  {cfgAzure(), "maa_endpoint"},
		"azure needs vm_name":       {cfgAzure(), "vm_name"},
	}
	for name, tc := range cases {
		c := tc.cfg
		c.TEEInfo = map[string]string{}
		for k, v := range tc.cfg.TEEInfo {
			if k != tc.drop {
				c.TEEInfo[k] = v
			}
		}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "tee_info."+tc.drop) {
			t.Errorf("%s: want tee_info.%s error, got %v", name, tc.drop, err)
		}
	}
	for _, c := range []BuildConfig{cfgAWS(), cfgGCP(), cfgAzure()} {
		if err := c.Validate(); err != nil {
			t.Errorf("%s: complete config must validate: %v", c.Platform, err)
		}
	}
	bad := cfgAWS()
	bad.Platform = "ibm"
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "platform") {
		t.Errorf("unknown platform must be rejected: %v", err)
	}
	bad = cfgAWS()
	bad.Source.Commit = ""
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "source") {
		t.Errorf("missing commit must be rejected: %v", err)
	}
}

// build.md — 플랫폼마다 측정값 기록에 넣어야 하는 것.
