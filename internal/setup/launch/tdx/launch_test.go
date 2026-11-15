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

// stockCfg is the shape of Ubuntu 24.04's grub.cfg on the GCP TDX guest with GRUB_FORCE_PARTUUID off: one stock
// 'Ubuntu' entry with a linux and an initrd line (docs/plan/app-fixture-recording.md, "The stock entry").
const stockCfg = `### BEGIN /etc/grub.d/10_linux ###
menuentry 'Ubuntu' --class ubuntu --class gnu-linux --class gnu --class os $menuentry_id_option 'gnulinux-simple-9d4d258d' {
	recordfail
	load_video
	gfxmode $linux_gfx_mode
	insmod gzio
	insmod part_gpt
	insmod ext2
	search --no-floppy --fs-uuid --set=root 9d4d258d-90ac-4d94-b296-007fa0812719
	linux	/boot/vmlinuz-7.0.0-1017-gcp root=UUID=9d4d258d-90ac-4d94-b296-007fa0812719 ro  console=ttyS0,115200
	initrd	/boot/initrd.img-7.0.0-1017-gcp
}
submenu 'Advanced options for Ubuntu' $menuentry_id_option 'gnulinux-advanced-9d4d258d' {
	menuentry 'Ubuntu, with Linux 7.0.0-1017-gcp' --class ubuntu {
		linux	/boot/vmlinuz-7.0.0-1017-gcp root=UUID=9d4d258d ro
		initrd	/boot/initrd.img-7.0.0-1017-gcp
	}
}
### END /etc/grub.d/10_linux ###
`

func TestEntry_ChangesOnlyTitleInitrdAndArgs(t *testing.T) {
	stock, err := StockEntry(stockCfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stock, "submenu") || !strings.HasSuffix(stock, "}\n") {
		t.Fatalf("stock entry not cut at its own closing brace:\n%s", stock)
	}
	got, err := Entry(stock, "attestamp", "initrd-attestamp.img", "attestamp.x=1")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(
		"menuentry 'Ubuntu' --class ubuntu --class gnu-linux --class gnu --class os $menuentry_id_option 'gnulinux-simple-9d4d258d' {", "menuentry 'attestamp' --id attestamp {",
		"initrd.img-7.0.0-1017-gcp", "initrd-attestamp.img",
		"console=ttyS0,115200", "console=ttyS0,115200 attestamp.x=1",
	).Replace(stock)
	if got != want {
		t.Fatalf("entry:\n%s\nwant:\n%s", got, want)
	}
}

func TestEntry_Refusals(t *testing.T) {
	stock, _ := StockEntry(stockCfg)
	initrdless := strings.Replace(stock, "\tinitrd\t/boot/initrd.img-7.0.0-1017-gcp\n", "", 1)
	if _, err := Entry(initrdless, "attestamp", "initrd-attestamp.img", ""); err == nil || !strings.Contains(err.Error(), "GRUB_FORCE_PARTUUID") {
		t.Errorf("an entry without an initrd: %v", err)
	}
	for _, c := range []struct{ id, file, args string }{
		{"Attestamp", "initrd-a.img", ""}, {"a b", "initrd-a.img", ""}, {"a", "../x.img", ""}, {"a", "i.img", "x'; reboot; '"}, {"a", "i.img", "a\nb"},
	} {
		if _, err := Entry(stock, c.id, c.file, c.args); err == nil {
			t.Errorf("accepted id %q file %q args %q", c.id, c.file, c.args)
		}
	}
	if _, err := StockEntry("menuentry 'Debian' {\n}\n"); err == nil {
		t.Error("a grub.cfg without the Ubuntu entry was accepted")
	}
}

// guestRoot makes a temporary root with the files install reads and writes. updateGrub stands in for update-grub: it
// appends the output of /etc/grub.d/41_* (everything after its two header lines) to grub.cfg.
func guestRoot(t *testing.T, forcePartuuid string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"boot/grub", "etc/grub.d", "etc/default/grub.d"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, grubCfgPath), []byte(stockCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if forcePartuuid != "" {
		if err := os.WriteFile(filepath.Join(root, forcePartuuidPath), []byte(forcePartuuid), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

type updateGrub struct{ root string }

func (u updateGrub) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if name != "update-grub" {
		return nil, errors.New("unexpected " + name)
	}
	scripts, _ := filepath.Glob(filepath.Join(u.root, "etc", "grub.d", "41_*"))
	cfg, _ := os.ReadFile(filepath.Join(u.root, grubCfgPath))
	for _, s := range scripts {
		b, _ := os.ReadFile(s)
		lines := strings.SplitN(string(b), "\n", 3)
		cfg = append(cfg, lines[2]...)
	}
	return nil, os.WriteFile(filepath.Join(u.root, grubCfgPath), cfg, 0o644)
}

func TestInstall_WritesEntryInitrdAndDefault(t *testing.T) {
	root := guestRoot(t, "#GRUB_FORCE_PARTUUID=0123-abcd\n") // commented out, as on our guests
	initrd := filepath.Join(t.TempDir(), "initrd-app.img")
	if err := os.WriteFile(initrd, []byte("070701 the built initrd"), 0o644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	res, err := Install{Root: root, Initrd: initrd, Runner: updateGrub{root}}.Run(context.Background(), &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if res.InitrdPath != "/boot/initrd-attestamp.img" {
		t.Fatalf("initrd path %s", res.InitrdPath)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "boot", "initrd-attestamp.img")); string(b) != "070701 the built initrd" {
		t.Fatal("initrd not copied to /boot")
	}
	if b, _ := os.ReadFile(filepath.Join(root, defaultCfgPath)); string(b) != "GRUB_DEFAULT=attestamp\n" {
		t.Fatalf("default: %q", b)
	}
	script, _ := os.ReadFile(filepath.Join(root, "etc", "grub.d", "41_attestamp"))
	if !strings.HasPrefix(string(script), "#!/bin/sh\nexec tail -n +3 $0\nmenuentry 'attestamp' --id attestamp {") {
		t.Fatalf("grub.d script:\n%s", script)
	}
}

func TestInstall_RefusesForcePartuuid(t *testing.T) {
	root := guestRoot(t, "GRUB_FORCE_PARTUUID=0123-abcd\n")
	initrd := filepath.Join(t.TempDir(), "i.img")
	os.WriteFile(initrd, []byte("x"), 0o644)
	_, err := Install{Root: root, Initrd: initrd, Runner: updateGrub{root}}.Run(context.Background(), &bytes.Buffer{})
	if !errors.Is(err, ErrForcePartuuid) {
		t.Fatalf("want ErrForcePartuuid, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "boot", "initrd-attestamp.img")); err == nil {
		t.Fatal("the initrd was copied although the install was refused")
	}
}

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
// replays to a real quote's RTMRs, so the passing path waits for a VM (docs/plan/cvm-port.md).
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
