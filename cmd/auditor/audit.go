package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	lether "attestamp/internal/chain/lookup/auditor/ether"
	"attestamp/internal/cli"
	cmp "attestamp/internal/core/compare/auditor"
)

// auditCmd runs the Auditor's part in one command: request an attestation document from its own TEE and verify it, compare it with the values I built and measured,
// register those values only if they match, then read the registration back and check it. The screen shows one sentence per step.
//
// --tee-addr needs --build-measurements on every platform (requireBuild): the attested values must equal the ones I built, or
// nothing is registered. With only --build-measurements, that file is registered as it is.
func auditCmd(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	teeAddr := fs.String("tee-addr", "", "my TEE's host:port (a Nitro enclave, or a TDX or SEV-SNP guest), from which an attestation is requested first")
	vendorTee := fs.String("vendor-tee", "", "the Vendor's TEE host:port. also ask it, and say whether it runs the same value I built. the value registered is always the one I built")
	format := fs.String("format", "", "expected document format: aws-nitro, intel-tdx, amd-sev-snp-initrd or amd-sev-snp-verity. default: build-config.json's platform with --config, else aws-nitro")
	timeout := fs.Duration("timeout", 30*time.Second, "RA request timeout")
	nonce := fs.String("nonce", "", "fixed nonce (e.g. for a test fixture). empty uses the one just sent")
	vendorNonce := fs.String("vendor-nonce", "", "with --vendor-tee: fixed nonce for the Vendor's document (e.g. for a test fixture). empty uses the one just sent")
	buildMeasurements := fs.String("build-measurements", "", "the value I built or measured myself (out/measurements.json). with --tee-addr the attested value must match it, or nothing is registered")
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	registry := fs.String("registry", "", "Registry contract address")
	signingKeyFile := fs.String("signing-key-file", "", "signing key file (hex): its signature authorises the registration, and the same key sends the transaction")
	platform := fs.String("platform", "", "platform value recorded in the registration. empty uses --format")
	config := fs.String("config", "", "the Vendor's build-config.json (receive puts it in the clone root): platform, service, version and build_env are taken from it, so --format/--platform, --service, --version and --arch are optional and only override it, with a warning")
	service := fs.String("service", "", "service name. empty takes it from --config, else from an existing registration of the same measurements (anyone's)")
	version := fs.String("version", "", "version. empty takes it from --config, else from an existing registration, as above")
	arch := fs.String("arch", "", "CPU architecture the image was built for and measured on (amd64|arm64). default: build-config.json's with --config, else this machine's. give the instance's if the measurements file came from another machine")
	expires := fs.Duration("expires", 90*24*time.Hour, "how long from now until the registration expires")
	trustedApprovers := fs.String("trusted-approvers", "", "trusted approver addresses, comma-separated. empty uses the approver fixed in the registry")
	outDir := fs.String("out-dir", "", "directory for the intermediate files (document, parsed value). empty uses a temporary directory, removed at the end")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// With --config, the format, service, version and arch come from it (a flag overrides it, with a one-line warning). Without it, the
	// format is aws-nitro, service and version are found in an existing registration below or asked for, and arch is this machine's.
	cfg, err := loadBuildConfig(*config)
	if err != nil {
		return err
	}
	resolvePlatform(cfg, "format", format)
	tee := cli.TEENoun(*format) // "enclave" on Nitro, "guest" on TDX and SEV-SNP
	// The record key includes the platform, so registering under one name and looking up under another finds nothing.
	if *platform == "" {
		*platform = *format
	}
	if cfg != nil {
		if err := resolveRegistration(cfg, service, version, arch); err != nil {
			return err
		}
	} else if *arch == "" {
		*arch = defaultArch()
	}
	// Values not given as options are asked for in turn, as register does; a plain run need not pass everything as flags.
	askMainnetRPC(rpc)
	ask(registry, "Registry address (0x…)", "")
	ask(signingKeyFile, "my signing key file", "")
	if *teeAddr == "" && *buildMeasurements == "" {
		ask(teeAddr, "my "+tee+" host:port (empty uses a measurements file)", "")
	}
	if *teeAddr == "" && *buildMeasurements == "" {
		ask(buildMeasurements, "build measurements file", "out/measurements.json")
	}
	if *rpc == "" || !common.IsHexAddress(*registry) || *signingKeyFile == "" {
		return errors.New("--rpc, --registry and --signing-key-file are required")
	}
	if *teeAddr == "" && *buildMeasurements == "" {
		return errors.New("--tee-addr or --build-measurements is required")
	}
	if err := requireBuild(*format, *buildMeasurements); err != nil {
		return err
	}
	if err := checkArch(*arch); err != nil {
		return err
	}
	quiet = true // the steps below call raRequest, raParse and register; print one line here instead of their output

	measPath := *buildMeasurements
	parsedPath := ""
	if *teeAddr != "" {
		dir := *outDir
		if dir == "" {
			tmp, err := os.MkdirTemp("", "att-auditor-audit-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			dir = tmp
		} else if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		docPath := filepath.Join(dir, "doc.cbor")
		parsedPath = filepath.Join(dir, "parsed.json")

		t := cli.Start(fmt.Sprintf("Asking my %s at %s for an attestation", tee, *teeAddr))
		if err := raRequest([]string{"--tee-addr", *teeAddr, "--out-file", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
			t.Fail("no answer")
			return err
		}
		t.Done(fmt.Sprintf("%s document, %d bytes", *format, fileSize(docPath)))
		cli.Why("an attestation is a document the TEE hardware signs on the spot, stating what is running; the fresh nonce inside proves it was made just now")

		t = cli.Start("Checking the attestation (signature chain to the platform root, nonce)")
		parseArgs := []string{"--doc", docPath, "--format", *format, "--out-file", parsedPath}
		if *nonce != "" {
			parseArgs = append(parseArgs, "--nonce", *nonce)
		}
		if err := raParse(parseArgs); err != nil {
			t.Fail("invalid")
			return err
		}
		t.Done("genuine")
		cli.Why("the signature chains up to the platform vendor's root certificate, so this really came from a TEE; the values below are what it runs")
		printPCRs(parsedPath)
		if measPath == "" {
			measPath = parsedPath
		}
	} else {
		fmt.Printf("No %s given: registering %s as it is, without checking it against a running %s\n", tee, measPath, tee)
	}

	values, err := readMeasurements(measPath)
	if err != nil {
		return err
	}
	if parsedPath != "" && parsedPath != measPath {
		t := cli.Start(fmt.Sprintf("Comparing what the %s attested with my own value (%s)", tee, measPath))
		parsed, err := readMeasurements(parsedPath)
		if err != nil {
			t.Fail("cannot read")
			return err
		}
		v := (cmp.Impl{}).Compare(parsed, values)
		if !v.Pass {
			t.Fail("they differ")
			for _, r := range v.Reason {
				cli.Errorln("    ", r)
			}
			return fmt.Errorf("the %s is not running what I built, so this audit is not registering anything", tee)
		}
		t.Done(fmt.Sprintf("identical: the %s runs exactly what I built", tee))
		cli.Why(fmt.Sprintf("this is the audit: the value I obtained myself and the value the running %s attests to are the same, so the %s runs what I built", tee, tee))
	}

	// Also ask what the Vendor is running right now. If it differs, say so; the value registered is the one I verified.
	vendorRunsSomethingElse := false
	if *vendorTee != "" {
		dir, err := os.MkdirTemp("", "att-auditor-vendor-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		vDoc, vParsed := filepath.Join(dir, "doc.cbor"), filepath.Join(dir, "parsed.json")
		t := cli.Start(fmt.Sprintf("Asking the Vendor's %s at %s what it is running", tee, *vendorTee))
		if err := raRequest([]string{"--tee-addr", *vendorTee, "--out-file", vDoc, "--format", *format, "--timeout", timeout.String()}); err != nil {
			t.Fail("no answer")
			return err
		}
		vArgs := []string{"--doc", vDoc, "--format", *format, "--out-file", vParsed}
		if *vendorNonce != "" {
			vArgs = append(vArgs, "--nonce", *vendorNonce)
		}
		if err := raParse(vArgs); err != nil {
			t.Fail("invalid attestation")
			return err
		}
		vendorValues, err := readMeasurements(vParsed)
		if err != nil {
			return err
		}
		v := (cmp.Impl{}).Compare(vendorValues, values)
		if v.Pass {
			t.Done(fmt.Sprintf("same: the Vendor's %s runs exactly what I built", tee))
		} else {
			vendorRunsSomethingElse = true
			t.Fail(fmt.Sprintf("the Vendor's %s runs something else", tee))
			for _, r := range v.Reason {
				cli.Errorln("    ", r)
			}
			printPCRs(vParsed)
		}
		cli.Why(fmt.Sprintf("the Vendor claims to run this source; the only way to know is to ask its %s and compare with what the source really builds to", tee))
	}
	reader := lether.Reader{RPC: *rpc, RegistryAddr: common.HexToAddress(*registry), Platform: *platform}

	if *service == "" || *version == "" {
		t := cli.Start("No service/version given: looking for an existing registration of the same value")
		ictx, icancel := context.WithTimeout(context.Background(), time.Minute)
		recordKeys, err := reader.FindRecordKeys(ictx, values)
		if err != nil {
			icancel()
			t.Fail("chain error")
			return err
		}
		resolved := false
		for _, k := range recordKeys {
			existing, err := reader.Lookup(ictx, k)
			if err != nil {
				continue
			}
			if *service == "" {
				*service = existing.Meta.Service
			}
			if *version == "" {
				*version = existing.Meta.Version
			}
			t.Done(fmt.Sprintf("%s %s (from %s)", existing.Meta.Service, existing.Meta.Version, k))
			resolved = true
			break
		}
		icancel()
		if !resolved {
			t.Fail("none: enter them by hand")
		}
		ask(service, "service name", "")
		ask(version, "version", "")
		if *service == "" || *version == "" {
			return errors.New("--service and --version are required")
		}
	}

	t := cli.Start(fmt.Sprintf("Registering %s %s (%s) on the chain, signed with my signing key", *service, *version, *arch))
	regArgs := []string{
		"--rpc", *rpc, "--registry", *registry, "--signing-key-file", *signingKeyFile,
		"--measurements", measPath, "--platform", *platform, "--arch", *arch,
		"--service", *service, "--version", *version, "--expires", expires.String(),
	}
	if *config != "" {
		regArgs = append(regArgs, "--config", *config) // the build env printed and recorded is the config's, as resolved above
	}
	recordKey, err := register(regArgs)
	if err != nil {
		t.Fail("rejected")
		return err
	}
	t.Done("registered")
	cli.Why("the value now sits on the public registry under my signature; anyone can look it up by measurement alone, without asking me")
	fmt.Printf("    record key  %s\n    tx          %s\n    block       %d\n    expires     %s\n    approver    %s\n    build env   %s\n",
		lastRegistered.RecordKey, lastRegistered.Tx, lastRegistered.Block, lastRegistered.Expires, lastRegistered.Approver, lastRegistered.BuildEnv)

	t = cli.Start("Reading the registration back from the chain and checking it")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	reg, err := reader.Lookup(ctx, recordKey)
	if err != nil {
		t.Fail("cannot read")
		return err
	}
	approvers, err := resolveTrustedApprovers(ctx, reader, *trustedApprovers)
	if err != nil {
		t.Fail("cannot read the approver")
		return err
	}
	v := (cmp.Impl{}).Check(values, reg, cmp.Clock(reg), approvers)
	if !v.Pass {
		t.Fail("something is off")
		for _, r := range v.Reason {
			cli.Errorln("    ", r)
		}
		cli.Errorln("FAIL")
		os.Exit(1)
	}
	t.Done("value, expiry and approver all check out")
	cli.Why("the same check a User will run: read the registration back and make sure it matches, is not expired, and was signed by the approver this registry trusts")
	cli.Okln("PASS: audited and registered. Users can now verify against registry ", *registry)
	fmt.Printf("this registration is for %s only. Put \"build_env\": {\"arch\": \"%s\", …} in measurement-record.json; a build for the other architecture measures differently and is registered on its own\n", *arch, *arch)
	if vendorRunsSomethingElse {
		return fmt.Errorf("%w: the registration above SUCCEEDED (record key %s on registry %s). Only the comparison "+
			"with the Vendor's %s at %s failed: it does not run this value, so a User verifying that %s against this "+
			"registry will be told not to trust it. Do not run audit again: the registration is already done, and a second "+
			"run ends with exit 5, already registered", errVendorDiffers, recordKey, *registry, tee, *vendorTee, tee)
	}
	return nil
}

// errVendorDiffers is audit's result when its own value was registered but the Vendor's TEE, asked with --vendor-tee,
// runs something else. It maps to exit code 12, so a script sees the mismatch without reading the output; until
// 2026-11-16 this case exited 0. It is not a failed audit: the registration stands.
var errVendorDiffers = errors.New("registered, but the Vendor's TEE runs something else")

// requireBuild refuses an audit that has no value the Auditor built itself, on every platform: the trust model is
// that the Auditor rebuilds, and without a build value audit would register whatever the TEE reports.
//
// Why it started on the CVMs: measured on 2026-11-16 (docs/results/sessions-2026-11-16/tdx-initrdless.md), a TDX guest
// booted without an initrd, so with no application measured at all, was registered that way and then verified as
// trusted. TDX evidence is the quote alone, so the parser cannot see that no initrd was loaded (SEV-SNP's can: its
// evidence carries the event log). On Nitro there is no such concrete gap (the enclave runs one EIF, all of it in PCR0),
// but the rule is the same everywhere, as the trust model is. This does not stop a register call made by hand: that is
// an Auditor vouching for what it did not build. To only look at what a TEE reports, use ra-request and ra-parse.
func requireBuild(format, buildMeasurements string) error {
	if buildMeasurements != "" {
		return nil
	}
	tee, doc, how := cli.TEENoun(format), "docs/build/aws-nitro.md", "build the source and pass its out/measurements.json"
	switch {
	case format == "intel-tdx":
		doc, how = "docs/build/tdx.md, \"The Auditor's half\"", "rebuild and launch on your own guest, and pass its out/measurements.json"
	case strings.HasPrefix(format, "amd-sev-snp"):
		doc, how = "docs/build/sev-snp.md, \"The Auditor's half\"", "rebuild and launch on your own guest, and pass its out/measurements.json"
	}
	return fmt.Errorf("refusing to audit without --build-measurements: without a value I rebuilt myself, this would "+
		"register whatever the %s reports, and on TDX a boot that never measured the application (one without an "+
		"initrd) was registered that way and then trusted. To audit, %s (%s). To only look at what a %s reports, use "+
		"ra-request and ra-parse, which register nothing", tee, how, doc, tee)
}

// fileSize returns the size of the file at path, or 0 if it cannot be read.
func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// printPCRs prints the measurements in the parsed-values file, indented, sorted by name (the order of the measurement
// hash and of ra-parse), in one column as wide as the longest name.
func printPCRs(path string) {
	values, err := readMeasurements(path)
	if err != nil {
		return
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	width := 5 // PCR0 and RTMR0 keep the column they always had
	for _, m := range values {
		width = max(width, len(m.Name))
	}
	for _, m := range values {
		fmt.Printf("    %-*s %x\n", width, m.Name, m.Value)
	}
}
