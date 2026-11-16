package tdx

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	buildtdx "attestamp/internal/setup/build/tdx"
	parsetdx "attestamp/internal/tee/parse/tdx"
)

// --- a constructed CC event log ---

type event struct {
	mr     uint32
	typ    uint32
	data   string
	digest []byte // nil: SHA-384 of the text after the GRUB prefix
}

const evIPL, evSeparator = 0x0D, 0x04

func sum384(b []byte) []byte { s := sha512.Sum384(b); return s[:] }

// ccel writes a TCG crypto-agile log with one SHA-384 bank, as a TDX guest's firmware does, and returns it with the
// RTMR0-2 values the events extend to.
func ccel(events []event) ([]byte, [3][]byte) {
	var b bytes.Buffer
	le := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	spec := new(bytes.Buffer)
	spec.WriteString("Spec ID Event03\x00")
	for _, v := range []any{uint32(0), uint8(0), uint8(2), uint8(0), uint8(2), uint32(1), uint16(0x000C), uint16(48), uint8(0)} {
		binary.Write(spec, binary.LittleEndian, v)
	}
	le(uint32(0))
	le(uint32(3)) // EV_NO_ACTION
	b.Write(make([]byte, 20))
	le(uint32(spec.Len()))
	b.Write(spec.Bytes())
	var rtmr [3][]byte
	for i := range rtmr {
		rtmr[i] = make([]byte, 48)
	}
	for _, e := range events {
		d := e.digest
		if d == nil {
			text := e.data
			for _, p := range []string{"grub_cmd: ", "kernel_cmdline: "} {
				text = strings.TrimPrefix(text, p)
			}
			d = sum384([]byte(text))
		}
		le(e.mr)
		le(e.typ)
		le(uint32(1))
		le(uint16(0x000C))
		b.Write(d)
		le(uint32(len(e.data) + 1))
		b.WriteString(e.data + "\x00")
		rtmr[e.mr-1] = sum384(append(append([]byte{}, rtmr[e.mr-1]...), d...))
	}
	b.Write(bytes.Repeat([]byte{0xFF}, 64)) // a CCEL is padded
	return b.Bytes(), rtmr
}

var builtInitrd = []byte("070701 the combined initrd")

func bootEvents(initrd []byte) []event {
	return []event{
		{mr: 1, typ: evSeparator, data: "firmware", digest: sum384([]byte("fw"))},
		{mr: 2, typ: evIPL, data: "shim and grub", digest: sum384([]byte("bootloader"))},
		{mr: 3, typ: evIPL, data: "grub_cmd: linux /boot/vmlinuz-7.0.0-1017-gcp root=UUID=9d4d ro"},
		{mr: 3, typ: evIPL, data: "/boot/vmlinuz-7.0.0-1017-gcp", digest: sum384([]byte("kernel"))},
		{mr: 3, typ: evIPL, data: "kernel_cmdline: /boot/vmlinuz-7.0.0-1017-gcp root=UUID=9d4d ro"},
		{mr: 3, typ: evIPL, data: "grub_cmd: initrd /boot/initrd-attestamp.img"},
		{mr: 3, typ: evIPL, data: "/boot/initrd-attestamp.img", digest: sum384(initrd)},
	}
}

func TestGrubFromLog_FindsTheMeasuredInitrd(t *testing.T) {
	raw, rtmr := ccel(bootEvents(builtInitrd))
	g, err := grubFromLog(raw, rtmr)
	if err != nil {
		t.Fatal(err)
	}
	if g.initrdPath != "/boot/initrd-attestamp.img" || !bytes.Equal(g.initrdDigest, sum384(builtInitrd)) || !strings.HasSuffix(g.cmdline, "root=UUID=9d4d ro") {
		t.Fatalf("%+v", g)
	}
	built := buildtdx.Value{InitrdSHA384: hex.EncodeToString(sum384(builtInitrd))}
	if err := compareInitrd(built, g); err != nil {
		t.Fatal(err)
	}
	other := buildtdx.Value{InitrdSHA384: hex.EncodeToString(sum384([]byte("a different initrd")))}
	if err := compareInitrd(other, g); !errors.Is(err, ErrInitrdMismatch) {
		t.Fatalf("a different build passed: %v", err)
	}
}

func TestGrubFromLog_Refusals(t *testing.T) {
	// A log that does not end at the quoted RTMR2.
	raw, rtmr := ccel(bootEvents(builtInitrd))
	rtmr[2] = sum384([]byte("another boot"))
	if _, err := grubFromLog(raw, rtmr); !errors.Is(err, ErrEventLog) {
		t.Errorf("a log that does not replay: %v", err)
	}
	// The initrd command's text relabelled, with the digest of the real one.
	evs := bootEvents(builtInitrd)
	evs[5].digest = sum384([]byte("initrd /boot/initrd.img-7.0.0-1017-gcp"))
	raw, rtmr = ccel(evs)
	if _, err := grubFromLog(raw, rtmr); !errors.Is(err, ErrEventLog) {
		t.Errorf("an initrd command whose text is not what was measured: %v", err)
	}
	// No initrd at all (an initrdless boot).
	raw, rtmr = ccel(bootEvents(builtInitrd)[:5])
	if _, err := grubFromLog(raw, rtmr); !errors.Is(err, ErrEventLog) {
		t.Errorf("a boot without an initrd: %v", err)
	}
	// Two initrd files.
	evs = bootEvents(builtInitrd)
	evs[5].data = "grub_cmd: initrd /boot/a.img /boot/b.img"
	evs[5].digest = nil
	raw, rtmr = ccel(evs)
	if _, err := grubFromLog(raw, rtmr); !errors.Is(err, ErrEventLog) {
		t.Errorf("two initrd files: %v", err)
	}
}

// recordedQuote returns a quote recorded on the GCP TDX guest and the REPORTDATA it was made for.
func recordedQuote(t *testing.T) ([]byte, []byte) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "tee", "parse", "tdx", "testdata")
	q, err := os.ReadFile(filepath.Join(dir, "gcp-c3-opt1-v1-quote.bin"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := os.ReadFile(filepath.Join(dir, "gcp-c3-opt1-v1-record.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(rec), "\n") {
		if v, ok := strings.CutPrefix(l, "report_data "); ok {
			rd, err := hex.DecodeString(strings.TrimSpace(v))
			if err != nil {
				t.Fatal(err)
			}
			return q, rd
		}
	}
	t.Fatal("no report_data in the record")
	return nil, nil
}

func checkSetup(t *testing.T, log []byte) (Check, string) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, filepath.Dir(ccelPath)), 0o755)
	os.WriteFile(filepath.Join(root, ccelPath), log, 0o644)
	value := filepath.Join(t.TempDir(), "build-value.json")
	b, _ := json.Marshal(buildtdx.Value{Format: "intel-tdx", Option: 1, InitrdSHA384: hex.EncodeToString(sum384(builtInitrd))})
	os.WriteFile(value, b, 0o644)
	return Check{Root: root, BuildValue: value, OutDir: filepath.Join(root, "out")}, root
}

// The check verifies the quote first, and then refuses an event log that is not this quote's. Without a guest no log
// replays to a real quote's RTMRs, so the passing path waits for a VM (docs/status/cvm-port.md).
func TestCheck_RecordedQuoteVerifiesThenForeignLogIsRefused(t *testing.T) {
	q, rd := recordedQuote(t)
	raw, _ := ccel(bootEvents(builtInitrd))
	c, _ := checkSetup(t, raw)
	c.nonce = rd
	c.Quote = func(n []byte) ([]byte, error) { return q, nil }
	var log bytes.Buffer
	_, err := c.Run(context.Background(), &log)
	if !errors.Is(err, ErrEventLog) {
		t.Fatalf("want ErrEventLog after a verified quote, got %v\n%s", err, log.String())
	}
	if !strings.Contains(log.String(), "[check 2/4]") {
		t.Fatalf("did not get past the quote:\n%s", log.String())
	}
}

func TestCheck_QuoteForAnotherNonceIsRefused(t *testing.T) {
	q, _ := recordedQuote(t)
	raw, _ := ccel(bootEvents(builtInitrd))
	c, _ := checkSetup(t, raw)
	c.nonce = bytes.Repeat([]byte{7}, 32)
	c.Quote = func(n []byte) ([]byte, error) { return q, nil }
	if _, err := c.Run(context.Background(), &bytes.Buffer{}); !errors.Is(err, parsetdx.ErrNonce) {
		t.Fatalf("want ErrNonce, got %v", err)
	}
}

func TestCheckMarker(t *testing.T) {
	root := t.TempDir()
	v1 := buildtdx.Value{Option: 1}
	if err := checkMarker(root, v1); !errors.Is(err, ErrNotUnpacked) {
		t.Errorf("no marker: %v", err)
	}
	os.MkdirAll(filepath.Join(root, "dev"), 0o755)
	os.WriteFile(filepath.Join(root, "dev", "attestamp-app-marker"), []byte("file app 17\nattestamp app v1\n"), 0o644)
	if err := checkMarker(root, v1); err != nil {
		t.Errorf("option 1 with the marker: %v", err)
	}
	rh := strings.Repeat("ab", 32)
	v2 := buildtdx.Value{Option: 2, RootHash: rh}
	if err := checkMarker(root, v2); !errors.Is(err, ErrNotUnpacked) {
		t.Errorf("option 2, marker without the root hash: %v", err)
	}
	os.WriteFile(filepath.Join(root, "dev", "attestamp-app-marker"), []byte("file app.img 4096\n"+rh+"\n"), 0o644)
	if err := checkMarker(root, v2); !errors.Is(err, ErrVerity) {
		t.Errorf("option 2 without a status: %v", err)
	}
	os.MkdirAll(filepath.Join(root, "run"), 0o755)
	os.WriteFile(filepath.Join(root, "run", "attestamp-status"), []byte("fail "+rh+"\n"), 0o644)
	if err := checkMarker(root, v2); !errors.Is(err, ErrVerity) {
		t.Errorf("option 2 with status fail: %v", err)
	}
	os.WriteFile(filepath.Join(root, "run", "attestamp-status"), []byte("ok "+rh+"\n"), 0o644)
	if err := checkMarker(root, v2); err != nil {
		t.Errorf("option 2 mounted: %v", err)
	}
}

func TestWriteMeasurements_ShapeRegisterReads(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.json")
	if err := writeMeasurements(p, map[string]string{"RTMR0": "01", "MRTD": "02"}); err != nil {
		t.Fatal(err)
	}
	var v struct{ Measurements map[string]string }
	b, _ := os.ReadFile(p)
	if err := json.Unmarshal(b, &v); err != nil || v.Measurements["MRTD"] != "02" || v.Measurements["RTMR0"] != "01" {
		t.Fatalf("%s %v", b, err)
	}
}
