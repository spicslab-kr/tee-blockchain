package tdx

import (
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

var execCommand = exec.CommandContext

// The Option 2 image settings. They are the ones internal/setup/build/cpio/testdata/make-verity-image.sh used for the
// images that came out byte-identical on the SEV-SNP and the TDX guest (docs/plan/test-matrix.md, footnote 9), and the
// root hash depends on every one of them. verity_test.go reads that script and fails if the two drift apart.
var mksquashfsFlags = []string{
	"-noappend", "-no-xattrs", "-all-root", "-mkfs-time", "0", "-all-time", "0",
	"-noI", "-noD", "-noF", "-noX", "-no-fragments", "-no-duplicates", "-processors", "1", "-b", "4096",
}

const (
	verityBlock = 4096
	veritySalt  = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff" // veritysetup picks a random one otherwise
	verityUUID  = "11111111-2222-3333-4444-555555555555"                             // likewise
	imageMode   = 0o755                                                              // every folder and file: 755 and 775 gave different root hashes
)

func veritysetupArgs(img, hash string) []string {
	return []string{"format", img, hash, "--hash=sha256",
		fmt.Sprintf("--data-block-size=%d", verityBlock), fmt.Sprintf("--hash-block-size=%d", verityBlock),
		"--salt=" + veritySalt, "--uuid=" + verityUUID}
}

var rootHashLine = regexp.MustCompile(`(?m)^Root hash:\s+([0-9a-f]{64})\s*$`)

// verityImage lays the tree out under opt/app in a staging folder, with every mode 755 and every time 0, makes a
// squashfs image of it, pads the image to whole 4096-byte blocks, and formats a dm-verity hash tree for it. The layout
// (the application under opt/app inside the image) is the one of the measured images.
func (b Build) verityImage(ctx context.Context, work string, tree []cpio.Entry) (image, hashTree []byte, rootHash string, err error) {
	stage, err := os.MkdirTemp(work, "stage-*")
	if err != nil {
		return nil, nil, "", err
	}
	if err := stageTree(stage, tree); err != nil {
		return nil, nil, "", err
	}
	img := filepath.Join(work, filepath.Base(stage)+".img")
	hash := filepath.Join(work, filepath.Base(stage)+".hash")
	if _, err := b.Runner.Run(ctx, "mksquashfs", append([]string{stage, img}, mksquashfsFlags...)...); err != nil {
		return nil, nil, "", err
	}
	if err := padToBlock(img, verityBlock); err != nil {
		return nil, nil, "", err
	}
	out, err := b.Runner.Run(ctx, "veritysetup", veritysetupArgs(img, hash)...)
	if err != nil {
		return nil, nil, "", err
	}
	m := rootHashLine.FindSubmatch(out)
	if m == nil {
		return nil, nil, "", fmt.Errorf("veritysetup format printed no root hash:\n%s", out)
	}
	if image, err = os.ReadFile(img); err != nil {
		return nil, nil, "", err
	}
	if hashTree, err = os.ReadFile(hash); err != nil {
		return nil, nil, "", err
	}
	return image, hashTree, string(m[1]), nil
}

// stageTree writes the tree under root/opt/app. Modes are set explicitly (not left to the umask) and every time is
// set to 0 after everything is written, deepest first, so that writing a child does not move its parent's time.
func stageTree(root string, tree []cpio.Entry) error {
	app := filepath.Join(root, "opt", "app")
	paths := []string{root, filepath.Join(root, "opt"), app}
	if err := os.MkdirAll(app, imageMode); err != nil {
		return err
	}
	for _, e := range tree {
		if e.Name == "." {
			continue
		}
		p := filepath.Join(app, filepath.FromSlash(e.Name))
		if e.Dir {
			if err := os.MkdirAll(p, imageMode); err != nil {
				return err
			}
		} else if err := os.WriteFile(p, e.Data, imageMode); err != nil {
			return err
		}
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.Count(paths[i], string(filepath.Separator)) > strings.Count(paths[j], string(filepath.Separator))
	})
	epoch := time.Unix(0, 0)
	for _, p := range paths {
		if err := os.Chmod(p, imageMode); err != nil {
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
