package tdx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeTools stands in for mksquashfs and veritysetup. The "image" it writes is a digest of everything about the staged
// tree that the real mksquashfs would put in the image (names, modes, times, content), so a one-byte change in the
// application, a mode or a time changes it, as it would change the real one. It also records the arguments.
type fakeTools struct {
	calls   [][]string
	badMode []string // staged paths whose mode or time was not the fixed one
}

func (f *fakeTools) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	switch {
	case name == "mksquashfs" && len(args) == 1:
		return []byte("mksquashfs version 4.6.1 (2023/03/25)\n"), nil
	case name == "veritysetup" && len(args) == 1:
		return []byte("veritysetup 2.7.0 flags: UDEV BLKID KEYRING\n"), nil
	case name == "mksquashfs":
		stage, img := args[0], args[1]
		h := sha256.New()
		err := filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(stage, p)
			if !info.ModTime().Equal(time.Unix(0, 0)) || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o755) {
				f.badMode = append(f.badMode, rel)
			}
			fmt.Fprintf(h, "%s %v %d|", filepath.ToSlash(rel), info.IsDir(), info.ModTime().Unix())
			if !info.IsDir() {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				h.Write(b)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return nil, os.WriteFile(img, append([]byte("hsqs"), h.Sum(nil)...), 0o644)
	case name == "veritysetup" && args[0] == "format":
		img, err := os.ReadFile(args[1])
		if err != nil {
			return nil, err
		}
		if len(img)%verityBlock != 0 {
			return nil, fmt.Errorf("image is %d bytes, not whole blocks", len(img))
		}
		tree := sha256.Sum256(append([]byte("tree"), img...))
		if err := os.WriteFile(args[2], tree[:], 0o644); err != nil {
			return nil, err
		}
		root := sha256.Sum256(append([]byte(veritySalt), tree[:]...))
		return []byte("VERITY header information for " + args[2] + "\nUUID:            \t" + verityUUID + "\nRoot hash:      \t" + hex.EncodeToString(root[:]) + "\n"), nil
	}
	return nil, fmt.Errorf("unexpected call %s %v", name, args)
}

// fixture writes an application tree and a base initrd whose length is not a multiple of 4, like Ubuntu's.
func fixture(t *testing.T) (appDir, base string) {
	t.Helper()
	dir := t.TempDir()
	appDir = filepath.Join(dir, "app")
	if err := os.MkdirAll(filepath.Join(appDir, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "app"), []byte("attestamp app v1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "etc", "app.conf"), []byte("listen 5000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base = filepath.Join(dir, "initrd.img-base")
	if err := os.WriteFile(base, bytes.Repeat([]byte{0x28, 0xb5, 0x2f}, 1001), 0o644); err != nil { // 3003 bytes, mod 4 = 3
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

func TestBuild_Option1_ArchiveBeforeBaseAndValue(t *testing.T) {
	appDir, base := fixture(t)
	res, log, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: Option1})
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	initrd, err := os.ReadFile(res.Initrd)
	if err != nil {
		t.Fatal(err)
	}
	baseBytes, _ := os.ReadFile(base)
	if !bytes.HasSuffix(initrd, baseBytes) || !bytes.HasPrefix(initrd, []byte("070701")) {
		t.Fatal("the initrd is not the archive followed by the base initrd")
	}
	if (len(initrd)-len(baseBytes))%512 != 0 {
		t.Fatalf("the archive is %d bytes, not padded to 512, so the base would not start aligned", len(initrd)-len(baseBytes))
	}
	for _, name := range []string{markerEntry, "opt/app/app", "opt/app/etc/app.conf"} {
		if !bytes.Contains(initrd, []byte(name+"\x00")) {
			t.Errorf("archive has no %s", name)
		}
	}
	var v Value
	b, _ := os.ReadFile(res.Record)
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if v.Format != "intel-tdx" || v.Option != 1 || v.InitrdSHA384 != sum384(initrd) || v.InitrdSize != len(initrd) || v.RootHash != "" {
		t.Fatalf("build value: %+v", v)
	}
}

// The same files built into two folders give the same bytes: nothing about the run (folder, time) is in the initrd.
func TestBuild_Option1_SameBytesFromTwoRuns(t *testing.T) {
	appDir, base := fixture(t)
	a, _, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: Option1})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // a different second, in case a time leaked in
	b, _, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: Option1})
	if err != nil {
		t.Fatal(err)
	}
	if a.Value.InitrdSHA384 != b.Value.InitrdSHA384 {
		t.Fatal("two builds of the same files differ")
	}
}

func TestBuild_Option2_ImageInArchiveWithFixedSettings(t *testing.T) {
	appDir, base := fixture(t)
	tools := &fakeTools{}
	res, log, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: Option2, Runner: tools})
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	if len(tools.badMode) > 0 {
		t.Fatalf("staged with a mode other than 755 or a time other than 0: %v", tools.badMode)
	}
	var sq, vs []string
	for _, c := range tools.calls {
		switch {
		case c[0] == "mksquashfs" && len(c) > 2:
			sq = c
		case c[0] == "veritysetup" && c[1] == "format":
			vs = c
		}
	}
	if strings.Join(sq[3:], " ") != strings.Join(mksquashfsFlags, " ") {
		t.Fatalf("mksquashfs flags: %v", sq[3:])
	}
	if !strings.Contains(strings.Join(vs, " "), "--salt="+veritySalt) || !strings.Contains(strings.Join(vs, " "), "--uuid="+verityUUID) {
		t.Fatalf("veritysetup without the fixed salt and uuid: %v", vs)
	}
	initrd, _ := os.ReadFile(res.Initrd)
	for _, name := range []string{"opt/app/app.img", "opt/app/app.hash", "opt/app/roothash", markerEntry} {
		if !bytes.Contains(initrd, []byte(name+"\x00")) {
			t.Errorf("archive has no %s", name)
		}
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(res.Value.RootHash) || !bytes.Contains(initrd, []byte(res.Value.RootHash+"\n")) {
		t.Fatalf("root hash %q not recorded or not carried in the archive", res.Value.RootHash)
	}
	if res.Value.Tools["mksquashfs"] == "" || res.Value.Tools["veritysetup"] == "" {
		t.Fatalf("tool versions not recorded: %v", res.Value.Tools)
	}
}

func TestVerify_PassesWhenNothingChanges(t *testing.T) {
	for _, opt := range []int{Option1, Option2} {
		appDir, base := fixture(t)
		_, log, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: opt, Runner: &fakeTools{}, Verify: true})
		if err != nil {
			t.Fatalf("option %d: %v\n%s", opt, err, log)
		}
		if !strings.Contains(log, "reproducible on this host") {
			t.Fatalf("option %d: no verdict in\n%s", opt, log)
		}
	}
}

// The pass criterion of Phase 3.6: one byte changed between the two builds fails the build, and the output says where.
func TestVerify_OneByteChangedFailsTheBuild(t *testing.T) {
	for _, tc := range []struct {
		opt  int
		want string // what the report must name
	}{
		{Option1, "/opt/app/app"},
		{Option2, "root hash"},
	} {
		appDir, base := fixture(t)
		flip := func() error {
			p := filepath.Join(appDir, "app")
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			b[len(b)-2] ^= 0x01 // "v1" -> "v0"
			return os.WriteFile(p, b, 0o755)
		}
		_, log, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: tc.opt, Runner: &fakeTools{}, Verify: true, betweenBuilds: flip})
		if !errors.Is(err, ErrNotReproducible) {
			t.Fatalf("option %d: want ErrNotReproducible, got %v\n%s", tc.opt, err, log)
		}
		if !strings.Contains(log, tc.want) {
			t.Fatalf("option %d: the report does not name %q:\n%s", tc.opt, tc.want, log)
		}
	}
}

func TestVerify_ChangedBaseInitrdIsNamed(t *testing.T) {
	appDir, base := fixture(t)
	change := func() error { return os.WriteFile(base, []byte("another base"), 0o644) }
	_, log, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: Option1, Verify: true, betweenBuilds: change})
	if !errors.Is(err, ErrNotReproducible) || !strings.Contains(log, "base initrd") {
		t.Fatalf("want a failure naming the base initrd, got %v\n%s", err, log)
	}
}

func TestBuild_Refusals(t *testing.T) {
	appDir, base := fixture(t)
	if _, err := (Build{AppDir: appDir, BaseInitrd: base, Option: Option1, goos: "windows"}).Run(context.Background(), &bytes.Buffer{}); !errors.Is(err, ErrNotLinux) {
		t.Errorf("windows: %v", err)
	}
	if _, _, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: 3}); err == nil {
		t.Error("option 3 accepted")
	}
	if _, _, err := run(t, Build{AppDir: appDir, Option: Option1}); err == nil {
		t.Error("no base initrd accepted")
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink("app", filepath.Join(appDir, "link")); err != nil {
			t.Fatal(err)
		}
		if _, _, err := run(t, Build{AppDir: appDir, BaseInitrd: base, Option: Option1}); err == nil {
			t.Error("a symbolic link in the application was accepted")
		}
	}
}

// The marker is the one the TDX fixtures were recorded with (cpio/testdata/mkappinitrd).
func TestMarker_SameAsTheRecorder(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "cpio", "testdata", "mkappinitrd", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(src, []byte("const markerScript = `"+MarkerScript+"`")) {
		t.Fatal("MarkerScript differs from mkappinitrd's markerScript")
	}
}

// The Option 2 settings are the ones of make-verity-image.sh, which made the images measured byte-identical on two
// guests. If one side changes, this fails, so the script stays a true record of what the build does.
func TestVeritySettings_SameAsTheScript(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "cpio", "testdata", "make-verity-image.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Join(strings.Fields(strings.ReplaceAll(string(b), "\\\n", " ")), " ")
	for _, want := range []string{
		strings.Join(mksquashfsFlags, " "),
		"--hash=sha256 --data-block-size=4096 --hash-block-size=4096",
		"--salt=" + veritySalt,
		"--uuid=" + verityUUID,
		"umask 022",
		`chmod 755 "$WORK" "$WORK/opt" "$WORK/opt/app" "$WORK/opt/app/app"`,
		"touch -h -d @0",
		"PAD=$(( (4096 - SZ % 4096) % 4096 ))",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("make-verity-image.sh has no %q", want)
		}
	}
}
