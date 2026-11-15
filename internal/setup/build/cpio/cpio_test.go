package cpio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// appTree is the layout an application archive has: /opt/app/app, executable. README is there to put a 0644 file and
// a size that is not a multiple of 4 into the archive.
func appTree() []Entry {
	return []Entry{
		{Name: ".", Dir: true},
		{Name: "opt", Dir: true},
		{Name: "opt/app", Dir: true},
		{Name: "opt/app/app", Executable: true, Data: []byte("attestamp-cpio-golden application\n")},
		{Name: "opt/app/README", Data: []byte("hi\n")},
	}
}

// flatTree has no subdirectories and only executable files. It is the one tree mkcpio.py also writes correctly on
// Windows, where os.path.relpath uses "\" and every existing file passes os.access(X_OK).
func flatTree() []Entry {
	return []Entry{
		{Name: ".", Dir: true},
		{Name: "app", Executable: true, Data: []byte("attestamp-cpio-golden application\n")},
		{Name: "b", Executable: true, Data: []byte("12345")},
	}
}

func mustBytes(t *testing.T, es []Entry) []byte {
	t.Helper()
	b, err := Bytes(es)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWrite_SameInputSameBytes(t *testing.T) {
	a, b := mustBytes(t, appTree()), mustBytes(t, appTree())
	if !bytes.Equal(a, b) {
		t.Fatal("two archives of the same tree differ")
	}
}

func TestWrite_OrderDoesNotMatter(t *testing.T) {
	es := appTree()
	reversed := make([]Entry, len(es))
	for i, e := range es {
		reversed[len(es)-1-i] = e
	}
	if !bytes.Equal(mustBytes(t, es), mustBytes(t, reversed)) {
		t.Fatal("the order entries are given in changed the archive")
	}
}

func TestWrite_OneByteChangesOutput(t *testing.T) {
	es := appTree()
	before := mustBytes(t, es)
	es[3].Data = append([]byte(nil), es[3].Data...)
	es[3].Data[0] ^= 0x01
	if bytes.Equal(before, mustBytes(t, es)) {
		t.Fatal("a one-byte change in a file did not change the archive")
	}
	es = appTree()
	es[3].Executable = false
	if bytes.Equal(before, mustBytes(t, es)) {
		t.Fatal("dropping the execute bit did not change the archive")
	}
}

// Read the fields back from the bytes, independently of Write.
func TestWrite_FixedHeaderFields(t *testing.T) {
	b := mustBytes(t, appTree())
	if len(b)%blockSize != 0 {
		t.Fatalf("archive is %d bytes, not a multiple of %d", len(b), blockSize)
	}
	field := func(off, i int) uint64 {
		v, err := strconv.ParseUint(string(b[off+6+8*i:off+6+8*i+8]), 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	wantNames := []string{".", "opt", "opt/app", "opt/app/README", "opt/app/app", trailerName}
	off := 0
	for n, want := range wantNames {
		if string(b[off:off+6]) != newcMagic {
			t.Fatalf("entry %d: magic %q", n, b[off:off+6])
		}
		ino, mode, uid, gid, nlink, mtime, size, nameSize := field(off, 0), field(off, 1), field(off, 2), field(off, 3), field(off, 4), field(off, 5), field(off, 6), field(off, 11)
		name := string(b[off+headerSize : off+headerSize+int(nameSize)-1])
		if name != want {
			t.Fatalf("entry %d is %q, want %q (sorted by name)", n, name, want)
		}
		if uid != 0 || gid != 0 || mtime != 0 {
			t.Errorf("%s: uid %d gid %d mtime %d, want all 0", name, uid, gid, mtime)
		}
		wantIno := uint64(n + 1)
		if name == trailerName {
			wantIno = 0
		}
		if ino != wantIno {
			t.Errorf("%s: ino %d, want %d", name, ino, wantIno)
		}
		switch name {
		case ".", "opt", "opt/app":
			if mode != modeDir || nlink != 2 {
				t.Errorf("%s: mode %o nlink %d", name, mode, nlink)
			}
		case "opt/app/app":
			if mode != modeExec || nlink != 1 {
				t.Errorf("%s: mode %o nlink %d", name, mode, nlink)
			}
		case "opt/app/README":
			if mode != modeFile || nlink != 1 {
				t.Errorf("%s: mode %o nlink %d", name, mode, nlink)
			}
		}
		off += headerSize + int(nameSize)
		off += pad(off, 4)
		off += int(size)
		off += pad(off, 4)
	}
	for _, c := range b[off:] {
		if c != 0 {
			t.Fatal("non-zero byte after the trailer")
		}
	}
}

func TestWrite_RefusesDuplicatesAndBadNames(t *testing.T) {
	if _, err := Bytes(append(appTree(), Entry{Name: "opt"})); err == nil {
		t.Error("a name given twice must be refused")
	}
	if _, err := Bytes([]Entry{{Name: `opt\app`}}); err == nil {
		t.Error("a Windows separator in a name must be refused")
	}
}

// The archives mkcpio.py wrote, kept as files. Each must be reproduced byte for byte. mkcpio.py itself, a script of the
// 2026-11 SEV-SNP runs, is no longer available (testdata/README.md), so these files are the only record of its output:
// they pin that this package still writes what the archives of those runs were.
func TestWrite_MatchesMkcpioGolden(t *testing.T) {
	for file, tree := range map[string][]Entry{
		"mkcpio-golden-flat.cpio": flatTree(),
		"mkcpio-golden-app.cpio":  appTree(),
	} {
		want, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		if got := mustBytes(t, tree); !bytes.Equal(got, want) {
			t.Errorf("%s: Go writes %s, mkcpio.py wrote %s", file, short(got), short(want))
		}
	}
}

// The same golden archive, reached through the files on disk: write the tree out, read it back with FromDir, and the
// archive must still be the one mkcpio.py wrote. (This replaces a comparison against mkcpio.py run live, removed on
// 2026-11-15 because the script no longer exists; that test skipped this part on Windows too.)
func TestFromDir_MatchesMkcpioGolden(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no execute bit on disk, so FromDir cannot tell 0755 from 0644 there")
	}
	tree, file := appTree(), "mkcpio-golden-app.cpio"
	want, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "root")
	for _, e := range tree {
		p := filepath.Join(root, filepath.FromSlash(e.Name))
		if e.Dir {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		mode := os.FileMode(0o644)
		if e.Executable {
			mode = 0o755
		}
		if err := os.WriteFile(p, e.Data, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil { // umask must not decide the mode
			t.Fatal(err)
		}
	}
	fromDisk, err := FromDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustBytes(t, fromDisk); !bytes.Equal(got, want) {
		t.Fatalf("%s: FromDir then Write gives %s, mkcpio.py wrote %s", file, short(got), short(want))
	}
}

func TestFromDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "opt", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "opt", "app", "app"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	es, err := FromDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]Entry{}
	for _, e := range es {
		names[e.Name] = e
	}
	for _, n := range []string{".", "opt", "opt/app", "opt/app/app"} {
		if _, ok := names[n]; !ok {
			t.Errorf("FromDir lacks %q: %v", n, es)
		}
	}
	if runtime.GOOS != "windows" && !names["opt/app/app"].Executable {
		t.Error("a 0755 file must read as executable")
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink("app", filepath.Join(root, "opt", "app", "link")); err != nil {
			t.Fatal(err)
		}
		if _, err := FromDir(root); err == nil {
			t.Error("a symbolic link must be refused")
		}
	}
}

func TestJoin(t *testing.T) {
	base, app := []byte("BASE"), []byte("APP")
	if got := string(Join(base, app, AppFirst)); got != "APPBASE" {
		t.Errorf("AppFirst = %q", got)
	}
	// a base whose length is a multiple of 4 gets no padding
	if got := string(Join(base, app, AppLast)); got != "BASEAPP" {
		t.Errorf("AppLast = %q", got)
	}
}

// The kernel accepts an uncompressed archive after a compressed segment only at a 4-byte-aligned offset. With AppLast
// the archive must start on one whatever the base's length, with zero bytes in between (which the kernel skips).
// Measured on GCP SEV-SNP: an Ubuntu base of 50100283 bytes (mod 4 = 3) appended without padding lost the archive.
func TestJoin_AppLastAlignsTheArchive(t *testing.T) {
	app := []byte("070701")
	for n := 0; n < 8; n++ {
		base := bytes.Repeat([]byte{0xFF}, n)
		got := Join(base, app, AppLast)
		at := len(got) - len(app)
		if at%4 != 0 || !bytes.Equal(got[at:], app) {
			t.Fatalf("base %d bytes: archive at offset %d", n, at)
		}
		if !bytes.Equal(got[:n], base) || !bytes.Equal(got[n:at], make([]byte, at-n)) || at-n > 3 {
			t.Fatalf("base %d bytes: want the base, then %d zero bytes, got %x", n, at-n, got[:at])
		}
	}
	// the base length of the measured Ubuntu initrd: 3 zero bytes
	base := make([]byte, 50100283)
	if got := Join(base, app, AppLast); len(got) != 50100284+len(app) {
		t.Fatalf("len = %d, want %d", len(got), 50100284+len(app))
	}
}

func short(b []byte) string {
	h := sha256.Sum256(b)
	return strconv.Itoa(len(b)) + " bytes sha256 " + hex.EncodeToString(h[:8])
}
