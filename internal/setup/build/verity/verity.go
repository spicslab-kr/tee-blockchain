// Package verity makes the read-only application image of Option 2: a squashfs image of the application tree and its
// dm-verity hash tree, with the settings that gave the same image and root hash on two machines (the SEV-SNP and the
// TDX guest, 2026-11-11; docs/plan/test-matrix.md, footnote 9). TDX carries the image inside the initrd, SEV-SNP on
// disk; both build it here, so the settings exist once.
//
// The settings are those of internal/setup/build/cpio/testdata/make-verity-image.sh, which made the measured images;
// verity_test.go reads that script and fails if the two drift apart. mksquashfs and veritysetup are run as tools, as
// the Nitro build runs docker and nitro-cli: no Go writer is known to produce the same bytes.
package verity

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"attestamp/internal/setup/build/cpio"
)

// MksquashfsFlags returns the mksquashfs options after the source and the image path.
func MksquashfsFlags() []string {
	return []string{
		"-noappend", "-no-xattrs", "-all-root", "-mkfs-time", "0", "-all-time", "0",
		"-noI", "-noD", "-noF", "-noX", "-no-fragments", "-no-duplicates", "-processors", "1", "-b", "4096",
	}
}

const (
	Block = 4096                                                               // squashfs block, dm-verity data and hash block; the image is padded to it
	Salt  = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff" // veritysetup picks a random one otherwise
	UUID  = "11111111-2222-3333-4444-555555555555"                             // likewise
	Mode  = 0o755                                                              // every folder and file: 755 and 775 gave different root hashes
)

// VeritysetupArgs returns the veritysetup arguments that format hash for img.
func VeritysetupArgs(img, hash string) []string {
	return []string{"format", img, hash, "--hash=sha256",
		fmt.Sprintf("--data-block-size=%d", Block), fmt.Sprintf("--hash-block-size=%d", Block),
		"--salt=" + Salt, "--uuid=" + UUID}
}

// Runner runs an external tool and returns its standard output and error together.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs the tool with os/exec.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s: %w\n%s", name, err, bytes.TrimSpace(out))
	}
	return out, nil
}

// Image is the made image.
type Image struct {
	Data     []byte // the squashfs image, padded to whole blocks (app.img)
	HashTree []byte // the dm-verity hash tree (app.hash)
	RootHash string // hex; what the boot checks the image against
}

// Tools returns the first line of each tool's version output, which the build records: the versions are part of
// what makes the bytes. A missing tool is an error naming the packages to install.
func Tools(ctx context.Context, r Runner) (map[string]string, error) {
	out := map[string]string{}
	for _, t := range [][]string{{"mksquashfs", "-version"}, {"veritysetup", "--version"}} {
		v, err := r.Run(ctx, t[0], t[1:]...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w (Option 2 needs squashfs-tools and cryptsetup: apt-get install squashfs-tools cryptsetup-bin)", t[0], err)
		}
		line, _, _ := strings.Cut(strings.TrimSpace(string(v)), "\n")
		out[t[0]] = line
	}
	return out, nil
}

var rootHashLine = regexp.MustCompile(`(?m)^Root hash:\s+([0-9a-f]{64})\s*$`)

// Make lays the tree out under opt/app in a staging folder inside work, with every mode 755 and every time 0, makes a
// squashfs image of it, pads the image to whole 4096-byte blocks, and formats a dm-verity hash tree for it. The layout
// (the application under opt/app inside the image) is the one of the measured images.
func Make(ctx context.Context, r Runner, work string, tree []cpio.Entry) (Image, error) {
	var im Image
	stage, err := os.MkdirTemp(work, "stage-*")
	if err != nil {
		return im, err
	}
	if err := stageTree(stage, tree); err != nil {
		return im, err
	}
	img := filepath.Join(work, filepath.Base(stage)+".img")
	hash := filepath.Join(work, filepath.Base(stage)+".hash")
	if _, err := r.Run(ctx, "mksquashfs", append([]string{stage, img}, MksquashfsFlags()...)...); err != nil {
		return im, err
	}
	if err := padToBlock(img, Block); err != nil {
		return im, err
	}
	out, err := r.Run(ctx, "veritysetup", VeritysetupArgs(img, hash)...)
	if err != nil {
		return im, err
	}
	m := rootHashLine.FindSubmatch(out)
	if m == nil {
		return im, fmt.Errorf("veritysetup format printed no root hash:\n%s", out)
	}
	if im.Data, err = os.ReadFile(img); err != nil {
		return im, err
	}
	if im.HashTree, err = os.ReadFile(hash); err != nil {
		return im, err
	}
	im.RootHash = string(m[1])
	return im, nil
}

// stageTree writes the tree under root/opt/app. Modes are set explicitly (not left to the umask) and every time is
// set to 0 after everything is written, deepest first, so that writing a child does not move its parent's time.
func stageTree(root string, tree []cpio.Entry) error {
	app := filepath.Join(root, "opt", "app")
	paths := []string{root, filepath.Join(root, "opt"), app}
	if err := os.MkdirAll(app, Mode); err != nil {
		return err
	}
	for _, e := range tree {
		if e.Name == "." {
			continue
		}
		p := filepath.Join(app, filepath.FromSlash(e.Name))
		if e.Dir {
			if err := os.MkdirAll(p, Mode); err != nil {
				return err
			}
		} else if err := os.WriteFile(p, e.Data, Mode); err != nil {
			return err
		}
		paths = append(paths, p)
	}
	sep := string(filepath.Separator)
	sort.Slice(paths, func(i, j int) bool { return strings.Count(paths[i], sep) > strings.Count(paths[j], sep) })
	epoch := time.Unix(0, 0)
	for _, p := range paths {
		if err := os.Chmod(p, Mode); err != nil {
			return err
		}
		if err := os.Chtimes(p, epoch, epoch); err != nil {
			return err
		}
	}
	return nil
}

// padToBlock appends zero bytes up to a multiple of block: dm-verity hashes whole data blocks.
func padToBlock(path string, block int64) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	pad := (block - st.Size()%block) % block
	if pad == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if _, err := f.Write(make([]byte, pad)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
