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
	"time"

	buildsnp "attestamp/internal/setup/build/snp"
	parsesnp "attestamp/internal/tee/parse/snp"
)

const fixtures = "../../../tee/parse/snp/testdata"

// recorded returns the evidence of a recording, packed as the responder sends it, and the nonce it was made for.
func recorded(t *testing.T, cell string) ([]byte, []byte) {
	t.Helper()
	var parts [3][]byte
	for i, suffix := range []string{"-report.bin", "-auxblob.bin", "-att.pb"} {
		b, err := os.ReadFile(filepath.Join(fixtures, "gcp-n2d-"+cell+suffix))
		if err != nil {
			t.Fatal(err)
		}
		parts[i] = b
	}
	ev, err := parsesnp.Encode(parts[0], parts[1], parts[2])
	if err != nil {
		t.Fatal(err)
	}
	rec, err := os.ReadFile(filepath.Join(fixtures, "gcp-n2d-"+cell+"-record.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(rec), "\n") {
		if v, ok := strings.CutPrefix(l, "nonce "); ok {
			n, err := hex.DecodeString(strings.TrimSpace(v))
			if err != nil {
				t.Fatal(err)
			}
			return ev, n
		}
	}
	t.Fatal("no nonce in the record")
	return nil, nil
}

// The build values those guests made, from their boot logs (gcp-n2d-opt1p-v1-boot.txt, gcp-n2d-opt2-v1-boot.txt).
func opt1Value(initrd string) buildsnp.Value {
	return buildsnp.Value{Format: buildsnp.FormatInitrd, Option: 1, InitrdSHA256: initrd}
}

func opt2Value(rootHash string) buildsnp.Value {
	return buildsnp.Value{Format: buildsnp.FormatVerity, Option: 2,
		InitrdSHA256: "b0fd172dff60ce1ba307d0318ae92d49d2dad8296ae7de8c912e9ab79dabf33a",
		RootHash:     rootHash, PCR12: hex.EncodeToString(buildsnp.PCR12(rootHash)), KernelArgs: buildsnp.RootHashArg + rootHash}
}

const (
	opt1V1Initrd = "a4504052832e43d87ccd5d1c9ee8e6560be7a2cd528be4a842e8d2c246f358bf"
	opt1V2Initrd = "332163d0c30f86c2e81ad48d7df15ab5af5a98eab286d3d8e06cce6b83cb6006"
	opt2V1Root   = "e29ec08585853c0fec829d6a5d7fd49d73dec6f4af88e5055752a52f6c3af0c9"
	opt2V2Root   = "a2f4df923b59d5ad7810b785af77146058253b06ba987b491047d6f610b951e1"
)

// check runs the post-boot check of one recording against a build value, on a temporary guest root holding the
// marker files the boot left.
func check(t *testing.T, cell string, v buildsnp.Value, marker map[string]string) (Measured, string, error) {
	t.Helper()
	ev, nonce := recorded(t, cell)
	root := t.TempDir()
	for p, content := range marker {
		os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o755)
		os.WriteFile(filepath.Join(root, p), []byte(content), 0o644)
	}
	value := filepath.Join(t.TempDir(), "build-value.json")
	b, _ := json.Marshal(v)
	os.WriteFile(value, b, 0o644)
	c := Check{Root: root, BuildValue: value, OutDir: filepath.Join(root, "out"), nonce: nonce,
		Evidence: func([]byte) ([]byte, error) { return ev, nil },
		parser:   parsesnp.Parser{Now: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)}}
	var log bytes.Buffer
	m, err := c.Run(context.Background(), &log)
	return m, log.String(), err
}

// The whole post-boot check, end to end on recorded evidence: the guests' own build values pass it.
func TestCheck_RecordedBootsPassWithTheirBuildValues(t *testing.T) {
	m, log, err := check(t, "opt1p-v1", opt1Value(opt1V1Initrd), map[string]string{"dev/attestamp-app-marker": "file app 17\nattestamp app v1\n"})
	if err != nil {
		t.Fatalf("Option 1: %v\n%s", err, log)
	}
	if m.Format != "amd-sev-snp-initrd" || m.Values["INITRD"] != opt1V1Initrd || len(m.Values) != 5 {
		t.Fatalf("Option 1: %+v", m)
	}
	var written struct{ Measurements map[string]string }
	b, _ := os.ReadFile(m.Measurements)
	if json.Unmarshal(b, &written) != nil || written.Measurements["CMDLINE"] != m.Values["CMDLINE"] {
		t.Fatalf("measurements.json: %s", b)
	}

	m, log, err = check(t, "opt2-v1", opt2Value(opt2V1Root), map[string]string{"run/attestamp-status": "ok\n"})
	if err != nil {
		t.Fatalf("Option 2: %v\n%s", err, log)
	}
	if m.Format != "amd-sev-snp-verity" || m.Values["PCR12"] != hex.EncodeToString(buildsnp.PCR12(opt2V1Root)) || len(m.Values) != 6 {
		t.Fatalf("Option 2: %+v", m)
	}
}

// Another build's values fail: v1's boot against v2's initrd, v1's image against v2's root hash.
func TestCheck_AnotherBuildIsRefused(t *testing.T) {
	if _, _, err := check(t, "opt1p-v1", opt1Value(opt1V2Initrd), map[string]string{"dev/attestamp-app-marker": "x"}); !errors.Is(err, ErrInitrdMismatch) {
		t.Errorf("Option 1, v2's initrd: %v", err)
	}
	if _, _, err := check(t, "opt2-v1", opt2Value(opt2V2Root), map[string]string{"run/attestamp-status": "ok\n"}); !errors.Is(err, ErrPCR12Mismatch) {
		t.Errorf("Option 2, v2's image: %v", err)
	}
}

func TestCheck_MarkersAndNonce(t *testing.T) {
	if _, _, err := check(t, "opt1p-v1", opt1Value(opt1V1Initrd), nil); !errors.Is(err, ErrNotUnpacked) {
		t.Errorf("Option 1 without the marker: %v", err)
	}
	if _, _, err := check(t, "opt2-v1", opt2Value(opt2V1Root), map[string]string{"run/attestamp-status": "fail\n"}); !errors.Is(err, ErrVerity) {
		t.Errorf("Option 2 with status fail: %v", err)
	}
	// Evidence made for another nonce (a replay) is refused before anything is compared.
	ev, _ := recorded(t, "opt1p-v1")
	value := filepath.Join(t.TempDir(), "v.json")
	b, _ := json.Marshal(opt1Value(opt1V1Initrd))
	os.WriteFile(value, b, 0o644)
	c := Check{Root: t.TempDir(), BuildValue: value, OutDir: t.TempDir(), nonce: bytes.Repeat([]byte{7}, 32),
		Evidence: func([]byte) ([]byte, error) { return ev, nil }, parser: parsesnp.Parser{Now: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)}}
	if _, err := c.Run(context.Background(), &bytes.Buffer{}); !errors.Is(err, parsesnp.ErrNonce) {
		t.Errorf("another nonce: %v", err)
	}
}

func TestInstall_Option2PutsImageAndRootHash(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"boot/grub", "etc/grub.d", "etc/default/grub.d"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "boot/grub/grub.cfg"), []byte("menuentry 'Ubuntu' --class ubuntu {\n\tlinux\t/boot/vmlinuz-7 root=UUID=x ro console=ttyS0,115200\n\tinitrd\t/boot/initrd.img-7\n}\n"), 0o644)
	out := t.TempDir()
	for _, f := range []string{buildsnp.InitrdFile, buildsnp.ImageFile, buildsnp.HashTreeFile} {
		os.WriteFile(filepath.Join(out, f), []byte(f), 0o644)
	}
	b, _ := json.Marshal(opt2Value(opt2V1Root))
	value := filepath.Join(out, buildsnp.ValueFile)
	os.WriteFile(value, b, 0o644)
	res, err := Install{BuildValue: value, Root: root, Runner: appendEntry{root}}.Run(context.Background(), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Entry, "attestamp.roothash="+opt2V1Root) {
		t.Fatalf("no root hash on the command line:\n%s", res.Entry)
	}
	for _, f := range []string{"app.img", "app.hash"} {
		if b, _ := os.ReadFile(filepath.Join(root, "var", "lib", "attestamp", f)); string(b) != f {
			t.Errorf("/var/lib/attestamp/%s not installed", f)
		}
	}
}

// appendEntry stands in for update-grub.
type appendEntry struct{ root string }

func (a appendEntry) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	s, _ := os.ReadFile(filepath.Join(a.root, "etc", "grub.d", "41_attestamp"))
	cfg, _ := os.ReadFile(filepath.Join(a.root, "boot", "grub", "grub.cfg"))
	parts := strings.SplitN(string(s), "\n", 3)
	return nil, os.WriteFile(filepath.Join(a.root, "boot", "grub", "grub.cfg"), append(cfg, parts[2]...), 0o644)
}
