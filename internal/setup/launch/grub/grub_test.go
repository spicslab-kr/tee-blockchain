package grub

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestInstall_PutsExtraFilesOnTheGuest(t *testing.T) {
	root := guestRoot(t, "")
	dir := t.TempDir()
	initrd, img := filepath.Join(dir, "i.img"), filepath.Join(dir, "app.img")
	os.WriteFile(initrd, []byte("070701"), 0o644)
	os.WriteFile(img, []byte("hsqs image"), 0o644)
	in := Install{Root: root, Initrd: initrd, ExtraArgs: "attestamp.roothash=ab", Runner: updateGrub{root},
		Files: []File{{Path: "/var/lib/attestamp/app.img", Src: img}}}
	res, err := in.Run(context.Background(), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "var", "lib", "attestamp", "app.img")); string(b) != "hsqs image" {
		t.Fatal("the image was not put on the guest")
	}
	if !strings.Contains(res.Entry, "console=ttyS0,115200 attestamp.roothash=ab") {
		t.Fatalf("root hash not on the command line:\n%s", res.Entry)
	}
	in.Files = []File{{Path: "../etc/passwd", Src: img}}
	if _, err := in.Run(context.Background(), &bytes.Buffer{}); err == nil {
		t.Fatal("a relative path was accepted")
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
