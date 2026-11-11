// Command mkappinitrd builds the initrd for an app-fixture recording (docs/plan/app-fixture-recording.md): a one-file
// dummy application archive written by internal/setup/build/cpio, joined with the guest's base initrd by cpio.Join.
//
// The application is deliberately trivial. What the recordings prove is not what the application is but that a
// changed application changes the measured value, so two runs differ in one byte of -app-text and nothing else.
//
//	mkappinitrd -base /boot/initrd.img-$(uname -r) -app-text "attestamp app v1" -placement first -out initrd-app.img
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

	"attestamp/internal/setup/build/cpio"
)

func main() {
	base := flag.String("base", "", "the guest's base initrd, e.g. /boot/initrd.img-$(uname -r)")
	appText := flag.String("app-text", "", "content of the dummy application file /opt/app/app (a newline is added)")
	placement := flag.String("placement", "", "first (Intel TDX, cpio.AppFirst) or last (AMD SEV-SNP, cpio.AppLast)")
	out := flag.String("out", "", "where to write the combined initrd")
	flag.Parse()
	if err := run(*base, *appText, *placement, *out); err != nil {
		fmt.Fprintln(os.Stderr, "mkappinitrd:", err)
		os.Exit(1)
	}
}

func run(basePath, appText, placement, out string) error {
	if basePath == "" || appText == "" || out == "" {
		return errors.New("-base, -app-text and -out are required")
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
	app, err := cpio.Bytes([]cpio.Entry{
		{Name: "opt", Dir: true},
		{Name: "opt/app", Dir: true},
		{Name: "opt/app/app", Data: []byte(appText + "\n")},
	})
	if err != nil {
		return err
	}
	joined := cpio.Join(base, app, where)
	if err := os.WriteFile(out, joined, 0o644); err != nil {
		return err
	}
	fmt.Printf("base      %s  %d bytes  sha256 %s  size mod 4 = %d\n", basePath, len(base), sum(base), len(base)%4)
	fmt.Printf("app       %q  %d bytes  sha256 %s\n", appText, len(app), sum(app))
	fmt.Printf("placement %s\n", placement)
	fmt.Printf("combined  %s  %d bytes  sha256 %s\n", out, len(joined), sum(joined))
	return nil
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
