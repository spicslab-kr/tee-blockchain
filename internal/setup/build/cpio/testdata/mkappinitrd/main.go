// Command mkappinitrd builds the initrd for an app-fixture recording (docs/plan/app-fixture-recording.md): an
// application archive written by internal/setup/build/cpio, joined with the guest's base initrd by cpio.Join.
//
// The application is either a one-line dummy file (-app-text, Option 1) or a directory tree copied in as it is
// (-app-dir, used on TDX for Option 2, where the squashfs image and its dm-verity hash tree go inside the initrd).
// Either way it lands under /opt/app in the initramfs.
//
// The archive also carries a marker: conf/conf.d/zz-attestamp-app-marker. initramfs-tools' /init sources every file in
// conf/conf.d (a plain glob, no ORDER file) after it has mounted the tmpfs on /run, and it later moves that /run onto
// the real root. The marker writes the names and sizes of the files in /opt/app, and the content of /opt/app/app, to
// /run/attestamp-app-marker. So after boot that file exists exactly when the kernel unpacked the archive, which a
// dmesg line cannot show reliably. (Checked against /usr/share/initramfs-tools/init on the guest before use; see the
// procedure.)
//
//	mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v1" -placement first -out initrd-app.img
//	mkappinitrd -base /boot/initrd.img-$(uname -r) -app-dir image-v1 -placement first -out initrd-app.img
//
// It prints the sizes and sha256 of the base, the archive and the result, and the base size modulo 4: the kernel only
// unpacks an archive appended after a compressed segment at a 4-byte-aligned offset (docs/build/tdx.md, "Before or
// after the base initrd").
//
// It is under testdata so that `go build ./...` skips it; build it explicitly:
//
//	GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o mkappinitrd ./internal/setup/build/cpio/testdata/mkappinitrd
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"

	"attestamp/internal/setup/build/cpio"
)

// markerScript is sourced by initramfs-tools' /init with ".", so it must not exit and must not fail.
const markerScript = `# attestamp app-fixture marker: proves that the application archive was unpacked into the initramfs.
if [ -d /opt/app ]; then
	{
		for f in /opt/app/*; do [ -f "$f" ] && echo "file ${f##*/} $(wc -c < "$f")"; done
		[ -f /opt/app/app ] && cat /opt/app/app
		[ -f /opt/app/roothash ] && cat /opt/app/roothash
	} > /run/attestamp-app-marker 2>/dev/null || true
fi
`

func main() {
	base := flag.String("base", "", "the guest's base initrd, e.g. /boot/initrd.img-$(uname -r)")
	appText := flag.String("app-text", "", "Option 1: content of the dummy application file /opt/app/app (a newline is added)")
	appDir := flag.String("app-dir", "", "instead of -app-text: a directory whose regular files are put under /opt/app (no subdirectories)")
	placement := flag.String("placement", "", "first (Intel TDX, cpio.AppFirst) or last (AMD SEV-SNP, cpio.AppLast)")
	out := flag.String("out", "", "where to write the combined initrd")
	flag.Parse()
	if err := run(*base, *appText, *appDir, *placement, *out); err != nil {
		fmt.Fprintln(os.Stderr, "mkappinitrd:", err)
		os.Exit(1)
	}
}

func run(basePath, appText, appDir, placement, out string) error {
	if basePath == "" || out == "" || (appText == "") == (appDir == "") {
		return errors.New("-base, -out and exactly one of -app-text and -app-dir are required")
	}
	var where cpio.Placement
	switch placement {
	case "first":
		where = cpio.AppFirst
	case "last":
		where = cpio.AppLast
	default:
		return errors.New("-placement must be first or last")
	}
	base, err := os.ReadFile(basePath)
	if err != nil {
		return err
	}
	entries := []cpio.Entry{
		{Name: "conf", Dir: true},
		{Name: "conf/conf.d", Dir: true},
		{Name: "conf/conf.d/zz-attestamp-app-marker", Data: []byte(markerScript)},
		{Name: "opt", Dir: true},
		{Name: "opt/app", Dir: true},
	}
	if appText != "" {
		entries = append(entries, cpio.Entry{Name: "opt/app/app", Data: []byte(appText + "\n")})
	} else {
		files, err := os.ReadDir(appDir)
		if err != nil {
			return err
		}
		for _, f := range files {
			if !f.Type().IsRegular() {
				return fmt.Errorf("%s: only regular files are archived, %s is not", appDir, f.Name())
			}
			data, err := os.ReadFile(path.Join(appDir, f.Name()))
			if err != nil {
				return err
			}
			entries = append(entries, cpio.Entry{Name: "opt/app/" + f.Name(), Data: data})
		}
	}
	app, err := cpio.Bytes(entries)
	if err != nil {
		return err
	}
	joined := cpio.Join(base, app, where)
	if err := os.WriteFile(out, joined, 0o644); err != nil {
		return err
	}
	what := fmt.Sprintf("%q", appText)
	if appDir != "" {
		what = appDir + "/"
	}
	fmt.Printf("base      %s  %d bytes  sha256 %s  size mod 4 = %d\n", basePath, len(base), sum(base), len(base)%4)
	fmt.Printf("app       %s  %d bytes  sha256 %s\n", what, len(app), sum(app))
	fmt.Printf("placement %s\n", placement)
	fmt.Printf("combined  %s  %d bytes  sha256 %s\n", out, len(joined), sum(joined))
	return nil
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
