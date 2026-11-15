package verity_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"attestamp/internal/setup/build/cpio"
	"attestamp/internal/setup/build/verity"
	"attestamp/internal/setup/build/verity/veritytest"
)

func TestMake_FixedSettingsAndLayout(t *testing.T) {
	f := &veritytest.Fake{}
	tree := []cpio.Entry{{Name: ".", Dir: true}, {Name: "app", Data: []byte("attestamp app v1\n")}, {Name: "etc", Dir: true}, {Name: "etc/a.conf", Data: []byte("x\n")}}
	im, err := verity.Make(context.Background(), f, t.TempDir(), tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.BadMode) > 0 {
		t.Fatalf("staged with a mode other than 755 or a time other than 0: %v", f.BadMode)
	}
	var sq, vs []string
	for _, c := range f.Calls {
		switch c[0] {
		case "mksquashfs":
			sq = c
		case "veritysetup":
			vs = c
		}
	}
	if strings.Join(sq[3:], " ") != strings.Join(verity.MksquashfsFlags(), " ") {
		t.Fatalf("mksquashfs flags: %v", sq[3:])
	}
	if !strings.Contains(strings.Join(vs, " "), "--salt="+verity.Salt) || !strings.Contains(strings.Join(vs, " "), "--uuid="+verity.UUID) {
		t.Fatalf("veritysetup without the fixed salt and uuid: %v", vs)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(im.RootHash) || len(im.Data)%verity.Block != 0 || len(im.HashTree) == 0 {
		t.Fatalf("image %d bytes, hash tree %d bytes, root hash %q", len(im.Data), len(im.HashTree), im.RootHash)
	}
}

// The settings are the ones of make-verity-image.sh, which made the images measured byte-identical on two guests. If
// one side changes, this fails, so the script stays a true record of what the builds do.
func TestSettings_SameAsTheScript(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "cpio", "testdata", "make-verity-image.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Join(strings.Fields(strings.ReplaceAll(string(b), "\\\n", " ")), " ")
	for _, want := range []string{
		strings.Join(verity.MksquashfsFlags(), " "),
		"--hash=sha256 --data-block-size=4096 --hash-block-size=4096",
		"--salt=" + verity.Salt,
		"--uuid=" + verity.UUID,
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
