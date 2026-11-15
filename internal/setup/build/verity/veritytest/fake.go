// Package veritytest has a stand-in for mksquashfs and veritysetup, for tests of the builds that make an Option 2
// image without the real tools.
package veritytest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"attestamp/internal/setup/build/verity"
)

// Fake stands in for mksquashfs and veritysetup. The "image" it writes is a digest of everything about the staged tree
// that the real mksquashfs would put in the image (names, modes, times, content), so a one-byte change in the
// application, a mode or a time changes it, as it would change the real one. It records the calls, and every staged
// path whose mode or time was not the fixed one.
type Fake struct {
	Calls   [][]string
	BadMode []string
}

func (f *Fake) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.Calls = append(f.Calls, append([]string{name}, args...))
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
			if !info.ModTime().Equal(time.Unix(0, 0)) || (runtime.GOOS != "windows" && info.Mode().Perm() != verity.Mode) {
				f.BadMode = append(f.BadMode, rel)
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
		if len(img)%verity.Block != 0 {
			return nil, fmt.Errorf("image is %d bytes, not whole blocks", len(img))
		}
		tree := sha256.Sum256(append([]byte("tree"), img...))
		if err := os.WriteFile(args[2], tree[:], 0o644); err != nil {
			return nil, err
		}
		root := sha256.Sum256(append([]byte(verity.Salt), tree[:]...))
		return []byte("VERITY header information for " + args[2] + "\nUUID:            \t" + verity.UUID + "\nRoot hash:      \t" + hex.EncodeToString(root[:]) + "\n"), nil
	}
	return nil, fmt.Errorf("unexpected call %s %v", name, args)
}
