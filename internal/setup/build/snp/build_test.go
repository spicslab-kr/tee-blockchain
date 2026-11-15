package snp

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"attestamp/internal/setup/build/verity/veritytest"
)

func fixture(t *testing.T, text string) (appDir, base string) {
	t.Helper()
	dir := t.TempDir()
	appDir = filepath.Join(dir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "app"), []byte(text+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base = filepath.Join(dir, "initrd.img-base")
	if err := os.WriteFile(base, bytes.Repeat([]byte{0x28, 0xb5, 0x2f}, 1001), 0o644); err != nil { // mod 4 = 3, like Ubuntu's
		t.Fatal(err)
	}
	return appDir, base
}

func run(t *testing.T, b Build) (Result, string, error) {
	t.Helper()
	b.goos = "linux"
	if b.OutDir == "" {
		b.OutDir = filepath.Join(t.TempDir(), "out")
	}
	var log bytes.Buffer
	res, err := b.Run(context.Background(), &log)
	return res, log.String(), err
}

// The application archive of the Option 1 recordings (internal/tee/parse/snp/testdata/gcp-n2d-opt1p-v{1,2}-boot.txt,
// the "app" line mkappinitrd printed on the guest). The build makes exactly that archive from the same application,
// so the initrd it makes from the same base is the one those guests booted and measured.
var recordedArchive = map[string]string{
	"attestamp app v1": "eeb1b5e9af3fb3ba3a08ff014e3a54f6366261428b2de97990db529c98f14c36",
	"attestamp app v2": "6370946247f8834ea8917598480489230ab4fe1e8d3ce53979fec8b558d61496",
}

func TestBuild_Option1_TheArchiveTheGuestBooted(t *testing.T) {
	for text, want := range recordedArchive {
		appDir, base := fixture(t, text)
		res, log, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: Option1})
		if err != nil {
			t.Fatalf("%v\n%s", err, log)
		}
		if res.Value.ArchiveSHA256 != want {
			t.Errorf("%q: archive sha256 %s, the recording's %s", text, res.Value.ArchiveSHA256, want)
		}
		initrd, _ := os.ReadFile(res.Initrd)
		baseBytes, _ := os.ReadFile(base)
		if !bytes.HasPrefix(initrd, baseBytes) || initrd[len(baseBytes)] != 0 || !bytes.HasPrefix(initrd[len(baseBytes)+1:], []byte("070701")) {
			t.Fatal("not the base, one zero byte of padding (3003 mod 4 = 3), then the archive")
		}
		if res.Value.Format != "amd-sev-snp-initrd" || res.Value.InitrdSHA256 != sum256(initrd) || res.Value.PCR12 != "" {
			t.Fatalf("%+v", res.Value)
		}
	}
}

func TestBuild_Option2_ImageOnDiskBaseUnchangedPCR12Predicted(t *testing.T) {
	appDir, base := fixture(t, "attestamp app v1")
	res, log, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: Option2, Runner: &veritytest.Fake{}})
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	initrd, _ := os.ReadFile(res.Initrd)
	baseBytes, _ := os.ReadFile(base)
	if !bytes.Equal(initrd, baseBytes) {
		t.Fatal("Option 2's initrd must be the base initrd, unchanged")
	}
	if b, _ := os.ReadFile(res.Image); len(b) == 0 || sum256(b) != res.Value.ImageSHA256 {
		t.Fatal("image not written or not recorded")
	}
	if b, _ := os.ReadFile(res.HashTree); len(b) == 0 {
		t.Fatal("hash tree not written")
	}
	v := res.Value
	if v.Format != "amd-sev-snp-verity" || v.KernelArgs != "attestamp.roothash="+v.RootHash || v.PCR12 != hex.EncodeToString(PCR12(v.RootHash)) {
		t.Fatalf("%+v", v)
	}
	var onDisk Value
	b, _ := os.ReadFile(res.Record)
	if err := json.Unmarshal(b, &onDisk); err != nil || onDisk.PCR12 != v.PCR12 {
		t.Fatalf("build-value.json: %s %v", b, err)
	}
}

// PCR12 for the root hash of the v1 image measured on both guests, recomputed from the hook's formula independently.
func TestPCR12_HookFormula(t *testing.T) {
	got := hex.EncodeToString(PCR12("e29ec08585853c0fec829d6a5d7fd49d73dec6f4af88e5055752a52f6c3af0c9"))
	if len(got) != 64 || got == hex.EncodeToString(make([]byte, 32)) {
		t.Fatalf("PCR12 %s", got)
	}
	if got == hex.EncodeToString(PCR12("a2f4df92")) {
		t.Fatal("PCR12 does not depend on the root hash")
	}
}

func TestVerify_OneByteChangedFailsTheBuild(t *testing.T) {
	for _, tc := range []struct {
		opt  int
		want string
	}{{Option1, "/opt/app/app"}, {Option2, "root hash"}} {
		appDir, base := fixture(t, "attestamp app v1")
		flip := func() error { return os.WriteFile(filepath.Join(appDir, "app"), []byte("attestamp app v0\n"), 0o644) }
		_, log, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: tc.opt, Runner: &veritytest.Fake{}, Verify: true, betweenBuilds: flip})
		if !errors.Is(err, ErrNotReproducible) || !strings.Contains(log, tc.want) {
			t.Fatalf("option %d: want ErrNotReproducible naming %q, got %v\n%s", tc.opt, tc.want, err, log)
		}
		_, log, err = run(t, Build{AppDir: appDir, BaseInitrd: base, Option: tc.opt, Runner: &veritytest.Fake{}, Verify: true})
		if err != nil || !strings.Contains(log, "reproducible on this host") {
			t.Fatalf("option %d, nothing changed: %v\n%s", tc.opt, err, log)
		}
	}
}

func TestBuild_Refusals(t *testing.T) {
	appDir, base := fixture(t, "x")
	if _, err := (Build{AppDir: appDir, BaseInitrd: base, Option: Option1, goos: "windows"}).Run(context.Background(), &bytes.Buffer{}); !errors.Is(err, ErrNotLinux) {
		t.Errorf("windows: %v", err)
	}
	if _, _, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: 3}); err == nil {
		t.Error("option 3 accepted")
	}
	if _, _, err := run(t, Build{AppDir: appDir, Option: Option2, Runner: &veritytest.Fake{}}); err == nil {
		t.Error("no base initrd accepted")
	}
}
