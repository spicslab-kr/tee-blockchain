package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"attestamp/internal/cli"
	snpbuild "attestamp/internal/setup/build/auditor/snp"
	snplaunch "attestamp/internal/setup/launch/auditor/snp"
)

// The --platform values that select the AMD SEV-SNP build and launch; the format decides the option.
const (
	snpInitrdFormat = snpbuild.FormatInitrd
	snpVerityFormat = snpbuild.FormatVerity
)

// buildSNP: build --platform amd-sev-snp-initrd | amd-sev-snp-verity (internal/setup/build/snp).
func buildSNP(format string, args []string) error {
	option, _ := snpbuild.OptionOf(format)
	fs := flag.NewFlagSet("build --platform "+format, flag.ContinueOnError)
	b := snpbuild.Build{Option: option}
	fs.StringVar(&b.AppDir, "app-dir", "", "the application: a directory of files")
	fs.StringVar(&b.BaseInitrd, "base-initrd", "", "the base initrd the Vendor published ("+snpVerityFormat+": built with the verity hook)")
	fs.StringVar(&b.OutDir, "out-dir", "out", "output directory")
	fs.BoolVar(&b.Verify, "verify", false, "build a second time and fail unless the bytes come out the same")
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
	cli.Okln("\n✓ ready: ", built.Initrd, "   build value: ", built.Record)
	fmt.Printf("next, as root on the SEV-SNP guest:  att-auditor launch --platform %s --build-value %s --reboot\n", built.Format, built.Record)
	return nil
}

// launchSNP: launch --platform amd-sev-snp-*. Without --check it installs the build under GRUB (and with --reboot
// reboots); with --check, after that reboot, it reads the measurements back and checks them against the build.
func launchSNP(format string, args []string) error {
	fs := flag.NewFlagSet("launch --platform "+format, flag.ContinueOnError)
	value := fs.String("build-value", filepath.Join("out", "build-value.json"), "the build's build-value.json; the initrd (and the image) are read from its folder")
	id := fs.String("entry-id", "", "GRUB entry id (default attestamp)")
	reboot := fs.Bool("reboot", false, "reboot into the new entry once it is installed")
	check := fs.Bool("check", false, "after the reboot: verify fresh evidence, compare INITRD (and PCR12) with the build and write measurements.json")
	out := fs.String("out-dir", "out", "where check writes measurements.json and the evidence")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	if *check {
		m, err := snplaunch.Check{BuildValue: *value, OutDir: *out}.Run(ctx, os.Stdout)
		if err != nil {
			return err
		}
		if m.Format != format {
			return fmt.Errorf("--platform %s, but the build value is %s", format, m.Format)
		}
		cli.Okln("\n✓ the boot measured the build. measurements: ", m.Measurements)
		fmt.Printf("next:  audit the Vendor's guest against these values (audit --format %s --build-measurements %s …)\n", m.Format, m.Measurements)
		return nil
	}
	if _, err := (snplaunch.Install{BuildValue: *value, ID: *id}).Run(ctx, os.Stdout); err != nil {
		return err
	}
	if !*reboot {
		fmt.Printf("next:  reboot, then  att-auditor launch --platform %s --check\n", format)
		return nil
	}
	fmt.Printf("rebooting into the new entry. after the boot:  att-auditor launch --platform %s --check\n", format)
	return exec.CommandContext(ctx, "systemctl", "reboot").Run()
}
