package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"attestamp/internal/cli"
	tdxbuild "attestamp/internal/setup/build/auditor/tdx"
	tdxlaunch "attestamp/internal/setup/launch/auditor/tdx"
)

// tdxFormat is the --platform value that selects the Intel TDX build and launch.
const tdxFormat = tdxbuild.Format

// platformOf returns the value of --platform in args (or -platform, with = or as the next argument) and the arguments
// without it, so build and launch can hand an intel-tdx run to its own flags and leave the Nitro flags as they were.
func platformOf(args []string) (string, []string) {
	var p string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if strings.HasPrefix(a, "-") && name == "platform" {
			if hasVal {
				p = val
			} else if i+1 < len(args) {
				p = args[i+1]
				i++
			}
			continue
		}
		rest = append(rest, a)
	}
	return p, rest
}

// buildTDX: build --platform intel-tdx. Builds the initrd that carries the application into a TDX guest
// (internal/setup/build/tdx). The four registered values come from launch, after one boot.
func buildTDX(args []string) error {
	fs := flag.NewFlagSet("build --platform intel-tdx", flag.ContinueOnError)
	b := tdxbuild.Build{}
	fs.StringVar(&b.AppDir, "app-dir", "", "the application: a directory of files, put under /opt/app")
	fs.StringVar(&b.BaseInitrd, "base-initrd", "", "the base initrd the Vendor published (Option 2: built with the dm-verity hook)")
	fs.IntVar(&b.Option, "option", 1, "1: the files as an archive in the initrd; 2: a squashfs + dm-verity image in the initrd")
	fs.StringVar(&b.OutDir, "out-dir", "out", "output directory")
	fs.BoolVar(&b.Verify, "verify", false, "build a second time and fail unless the initrd comes out byte for byte the same")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if b.AppDir == "" || b.BaseInitrd == "" {
		return errors.New("--app-dir and --base-initrd are required")
	}
	built, err := b.Run(context.Background(), os.Stdout)
	if err != nil {
		return err
	}
	cli.Okln("\n✓ initrd ready: ", built.Initrd, "   build value: ", built.Record)
	fmt.Printf("next, as root on the TDX guest:  att-auditor launch --platform %s --initrd %s --reboot\n", tdxbuild.Format, built.Initrd)
	return nil
}

// launchTDX: launch --platform intel-tdx. Without --check it installs the initrd under GRUB (and with --reboot
// reboots); with --check, run after that reboot, it reads the measurements back and checks them against the build.
func launchTDX(args []string) error {
	fs := flag.NewFlagSet("launch --platform intel-tdx", flag.ContinueOnError)
	in := tdxlaunch.Install{}
	fs.StringVar(&in.Initrd, "initrd", "", "the built initrd (install)")
	fs.StringVar(&in.ID, "entry-id", "", "GRUB entry id (default attestamp)")
	fs.StringVar(&in.ExtraArgs, "kernel-args", "", "arguments appended to the kernel command line; they are measured into RTMR2")
	reboot := fs.Bool("reboot", false, "reboot into the new entry once it is installed")
	check := fs.Bool("check", false, "after the reboot: verify a fresh quote, replay the event log, compare the measured initrd with the build and write measurements.json")
	c := tdxlaunch.Check{}
	fs.StringVar(&c.BuildValue, "build-value", filepath.Join("out", "build-value.json"), "the build's build-value.json (check)")
	fs.StringVar(&c.OutDir, "out-dir", "out", "where check writes measurements.json and the quote")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	if *check {
		m, err := c.Run(ctx, os.Stdout)
		if err != nil {
			return err
		}
		cli.Okln("\n✓ the boot measured the built initrd. measurements: ", m.Measurements)
		fmt.Println("next:  audit the Vendor's guest against these values (audit --format intel-tdx --build-measurements " + m.Measurements + " …)")
		return nil
	}
	if in.Initrd == "" {
		return errors.New("--initrd is required (or --check after the reboot)")
	}
	if _, err := in.Run(ctx, os.Stdout); err != nil {
		return err
	}
	if !*reboot {
		fmt.Println("next:  reboot, then  att-auditor launch --platform intel-tdx --check")
		return nil
	}
	fmt.Println("rebooting into the new entry. after the boot:  att-auditor launch --platform intel-tdx --check")
	return exec.CommandContext(ctx, "systemctl", "reboot").Run()
}
