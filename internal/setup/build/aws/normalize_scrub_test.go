package aws

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAt(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScrubBuildScratch(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, "tmp/MjFVGt80Zx", "2797\n")
	writeAt(t, root, "tmp/sub/deeper", "x")
	writeAt(t, root, "var/tmp/leftover", "x")
	writeAt(t, root, "etc/nginx/nginx.conf", "keep") // places that are not scratch are left alone
	if err := os.MkdirAll(filepath.Join(root, "run"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := runner{log: io.Discard}
	got := scrubBuildScratch(context.Background(), r, root)
	if len(got) != 2 || got[0] != "tmp" || got[1] != "var/tmp" {
		t.Fatalf("scrubbed = %v", got)
	}
	for _, d := range []string{"tmp", "var/tmp"} {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(d)))
		if err != nil {
			t.Fatalf("%s must still exist as a directory: %v", d, err)
		}
		if len(entries) != 0 {
			t.Errorf("%s must be empty, has %d entries", d, len(entries))
		}
	}
	if _, err := os.Stat(filepath.Join(root, "etc", "nginx", "nginx.conf")); err != nil {
		t.Errorf("non-scratch file must survive: %v", err)
	}
	if again := scrubBuildScratch(context.Background(), r, root); len(again) != 0 {
		t.Errorf("second pass scrubbed %v, want nothing", again)
	}
}

const sampleInstalled = `C:Q1abcabcabcabcabcabcabcabcabcabc=
P:musl
V:1.2.5-r10
A:x86_64
S:1234

C:Q1rxIOXX5GXNHXAW82AjWS1SDhLck=
P:.haproxy-rundeps
V:20260927.082249
A:x86_64
S:0
I:0
T:virtual meta package
D:ca-certificates so:libc.musl-x86_64.so.1

C:Q1zzzzzzzzzzzzzzzzzzzzzzzzzzzz=
P:pcre2
V:10.43-r1
A:x86_64
`

func TestFixApkVirtualVersions(t *testing.T) {
	root := t.TempDir()
	world := writeAt(t, root, "etc/apk/world", "alpine-baselayout\n.haproxy-rundeps=20260927.082249\nbusybox\n")
	installed := writeAt(t, root, "lib/apk/db/installed", sampleInstalled)

	r := runner{log: io.Discard}
	got := fixApkVirtualVersions(context.Background(), r, root)
	if len(got) != 2 || got[0] != "etc/apk/world" || got[1] != "lib/apk/db/installed" {
		t.Fatalf("fixed = %v", got)
	}

	w, _ := os.ReadFile(world)
	if string(w) != "alpine-baselayout\n.haproxy-rundeps=0\nbusybox\n" {
		t.Errorf("world:\n%s", w)
	}
	in, _ := os.ReadFile(installed)
	s := string(in)
	if strings.Contains(s, "20260927.082249") || strings.Contains(s, "Q1rxIOXX5GXNHXAW82AjWS1SDhLck=") {
		t.Errorf("virtual package's time-derived version/identity must be gone:\n%s", s)
	}
	if !strings.Contains(s, "P:.haproxy-rundeps\nV:0\n") || !strings.Contains(s, apkFixedIdentity+"\nP:.haproxy-rundeps") {
		t.Errorf("virtual package must get V:0 and the fixed identity:\n%s", s)
	}
	// Real packages are left alone.
	if !strings.Contains(s, "C:Q1abcabcabcabcabcabcabcabcabcabc=\nP:musl\nV:1.2.5-r10") || !strings.Contains(s, "C:Q1zzzzzzzzzzzzzzzzzzzzzzzzzzzz=\nP:pcre2\nV:10.43-r1") {
		t.Errorf("real packages must be untouched:\n%s", s)
	}

	if again := fixApkVirtualVersions(context.Background(), r, root); len(again) != 0 {
		t.Errorf("second pass changed %v, want nothing", again)
	}
}

func TestFixApkVirtualVersions_NoApk(t *testing.T) {
	root := t.TempDir() // without apk, as on a debian image, nothing is done
	if got := fixApkVirtualVersions(context.Background(), runner{log: io.Discard}, root); len(got) != 0 {
		t.Fatalf("fixed = %v, want nothing", got)
	}
}

// The nginx alpine-slim /etc/shadow as built on 2026-11-12 and on 2026-11-15 (day 20769 vs 20772): after the fix the
// two files are the same, and every other field is unchanged.
func TestFixShadowDates_TwoBuildDaysBecomeOne(t *testing.T) {
	day := func(d string) string {
		return "root:*::0:::::\nbin:!::0:::::\nnobody:!::0:::::\nnginx:!:" + d + ":0:99999:7:::\n"
	}
	var got [2][]byte
	for i, d := range []string{"20769", "20772"} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "etc", "shadow"), []byte(day(d)), 0o640); err != nil {
			t.Fatal(err)
		}
		if fixed := fixShadowDates(context.Background(), runner{log: io.Discard}, root); len(fixed) != 1 || fixed[0] != "etc/shadow" {
			t.Fatalf("fixed %v", fixed)
		}
		got[i], _ = os.ReadFile(filepath.Join(root, "etc", "shadow"))
	}
	want := "root:*::0:::::\nbin:!::0:::::\nnobody:!::0:::::\nnginx:!::0:99999:7:::\n"
	if string(got[0]) != want || string(got[1]) != want {
		t.Fatalf("got\n%s\n%s\nwant\n%s", got[0], got[1], want)
	}
	// Already empty: nothing to change, and a file without shadow lines is left alone.
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "etc"), 0o755)
	os.WriteFile(filepath.Join(root, "etc", "shadow"), []byte(want), 0o640)
	if fixed := fixShadowDates(context.Background(), runner{log: io.Discard}, root); len(fixed) != 0 {
		t.Fatalf("changed an already normalised file: %v", fixed)
	}
}

func TestStripBuildCaches(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, "var/cache/ldconfig/aux-cache", "x")
	writeAt(t, root, "root/.wget-hsts", "x")
	writeAt(t, root, "root/.cache/pip/http/abc", "x")
	writeAt(t, root, "etc/ld.so.cache", "keep") // content-based cache stays
	got := stripBuildCaches(context.Background(), runner{log: io.Discard}, root)
	if len(got) != 3 {
		t.Fatalf("removed = %v", got)
	}
	for _, p := range []string{"var/cache/ldconfig/aux-cache", "root/.wget-hsts", "root/.cache"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			t.Errorf("%s still there", p)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "etc", "ld.so.cache")); err != nil {
		t.Errorf("ld.so.cache must survive: %v", err)
	}
	if again := stripBuildCaches(context.Background(), runner{log: io.Discard}, root); len(again) != 0 {
		t.Errorf("second pass removed %v", again)
	}
}
