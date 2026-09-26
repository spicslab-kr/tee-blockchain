package aws

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestStripBuildLogs(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("var/log/apk.log")
	mk("var/log/apt/history.log")
	mk("var/log/nginx/access.log") // 소프트웨어 자체의 로그 — 남아야 한다
	mk("etc/apk/world")            // 무엇을 깔았나 — 남아야 한다

	r := runner{log: io.Discard}
	removed := stripBuildLogs(context.Background(), r, root)
	if len(removed) != 2 || removed[0] != "var/log/apk.log" || removed[1] != "var/log/apt" {
		t.Fatalf("removed = %v", removed)
	}
	for _, keep := range []string{"var/log/nginx/access.log", "etc/apk/world"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(keep))); err != nil {
			t.Errorf("%s must survive: %v", keep, err)
		}
	}
	for _, gone := range []string{"var/log/apk.log", "var/log/apt"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(gone))); err == nil {
			t.Errorf("%s must be removed", gone)
		}
	}
	if again := stripBuildLogs(context.Background(), r, root); len(again) != 0 {
		t.Errorf("second pass removed %v, want nothing", again)
	}
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{"My App": "my-app", "example-ollama": "example-ollama", "///": "app", "a.b_c": "a.b_c"} {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadPCR(t *testing.T) {
	m, err := ReadPCR([]byte(`{"Measurements":{"HashAlgorithm":"Sha384","PCR0":"aa","PCR1":"bb","PCR2":"cc"}}`))
	if err != nil || m["PCR0"] != "aa" || m["PCR2"] != "cc" {
		t.Fatalf("ReadPCR: %v %v", m, err)
	}
	if _, err := ReadPCR([]byte("not json")); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
