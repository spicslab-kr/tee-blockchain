package tdx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"attestamp/internal/cli"
)

// Paths on the guest, relative to Install.Root / Check.Root.
const (
	grubCfgPath       = "boot/grub/grub.cfg"
	forcePartuuidPath = "etc/default/grub.d/40-force-partuuid.cfg"
	defaultCfgPath    = "etc/default/grub.d/99-attestamp.cfg"
)

// DefaultID is the GRUB entry id, and the initrd's name in /boot is initrd-<id>.img.
const DefaultID = "attestamp"

// Install puts the built initrd in /boot and makes a GRUB entry for it the default. It does not reboot.
type Install struct {
	Root      string // the guest's root, / unless a test sets another
	Initrd    string // the built initrd (internal/setup/build/tdx, out/initrd-app.img)
	ID        string // GRUB entry id; DefaultID when empty
	ExtraArgs string // appended to the kernel command line; measured into RTMR2 like the rest of it
	Runner    Runner // runs update-grub; nil runs it with os/exec
}

// Installed is what Install wrote.
type Installed struct {
	ID         string
	InitrdPath string // as GRUB will name it, e.g. /boot/initrd-attestamp.img
	Entry      string // the menuentry block
}

// Runner runs a command on the guest and returns its output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs the command with os/exec.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s: %w\n%s", name, err, bytes.TrimSpace(out))
	}
	return out, nil
}

// ErrForcePartuuid: the guest boots without an initrd, so the application could never be loaded.
var ErrForcePartuuid = errors.New("GRUB_FORCE_PARTUUID is set")

// Run installs. The steps print what they do and why, as Nitro's launch does.
func (in Install) Run(ctx context.Context, log io.Writer) (Installed, error) {
	var res Installed
	if in.Root == "" {
		in.Root = "/"
		if os.Geteuid() != 0 {
			return res, errors.New("install writes to /boot and /etc and runs update-grub: run it as root")
		}
	}
	if in.ID == "" {
		in.ID = DefaultID
	}
	if in.Runner == nil {
		in.Runner = ExecRunner{}
	}
	initrd, err := os.ReadFile(in.Initrd)
	if err != nil {
		return res, err
	}
	step := func(n int, what, why string) {
		fmt.Fprintf(log, "\n%s %s…\n             %s\n", cli.Green(fmt.Sprintf("[launch %d/4]", n)), what, why)
	}

	step(1, "Checking that this guest boots with an initrd", "Ubuntu's cloud images set GRUB_FORCE_PARTUUID and boot without one; the application is in the initrd, so it would never be loaded")
	if err := checkForcePartuuid(filepath.Join(in.Root, forcePartuuidPath)); err != nil {
		return res, err
	}
	cfg, err := os.ReadFile(filepath.Join(in.Root, grubCfgPath))
	if err != nil {
		return res, err
	}
	stock, err := StockEntry(string(cfg))
	if err != nil {
		return res, err
	}
	file := "initrd-" + in.ID + ".img"
	if res.Entry, err = Entry(stock, in.ID, file, in.ExtraArgs); err != nil {
		return res, err
	}
	res.ID, res.InitrdPath = in.ID, "/boot/"+file

	step(2, "Copying the initrd to "+res.InitrdPath, "GRUB reads it from /boot and measures its SHA-384 into RTMR2 as it loads it")
	if err := os.WriteFile(filepath.Join(in.Root, "boot", file), initrd, 0o644); err != nil {
		return res, err
	}

	step(3, fmt.Sprintf("Writing the GRUB entry %q and making it the default", in.ID), "the stock entry with only its title and initrd changed; the default, not a one-shot grub-reboot, which a maintenance restart would lose")
	if err := os.WriteFile(filepath.Join(in.Root, "etc", "grub.d", "41_"+in.ID), []byte(grubScript(res.Entry)), 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(filepath.Join(in.Root, defaultCfgPath), []byte("GRUB_DEFAULT="+in.ID+"\n"), 0o644); err != nil {
		return res, err
	}
	fmt.Fprint(log, res.Entry)

	step(4, "Running update-grub", "it writes /boot/grub/grub.cfg from /etc/grub.d")
	if _, err := in.Runner.Run(ctx, "update-grub"); err != nil {
		return res, err
	}
	cfg, err = os.ReadFile(filepath.Join(in.Root, grubCfgPath))
	if err != nil {
		return res, err
	}
	if !strings.Contains(string(cfg), fmt.Sprintf("menuentry '%s' --id %s {", in.ID, in.ID)) {
		return res, fmt.Errorf("update-grub ran, but grub.cfg has no entry %q", in.ID)
	}
	fmt.Fprintln(log, cli.Green("✓ installed")+": reboot into it, then run the check (launch --check) to read the measurements")
	return res, nil
}

// checkForcePartuuid refuses when the file sets GRUB_FORCE_PARTUUID on a line that is not commented out.
func checkForcePartuuid(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if l := strings.TrimSpace(s.Text()); strings.HasPrefix(l, "GRUB_FORCE_PARTUUID=") {
			return fmt.Errorf("%w in %s: this guest boots without an initrd, so the application would never be loaded. Comment the line out (sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' %s) and run update-grub; docs/build/tdx.md explains the trade-off", ErrForcePartuuid, path, path)
		}
	}
	return s.Err()
}
