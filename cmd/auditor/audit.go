package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"

	lether "attestamp/internal/chain/lookup/auditor/ether"
	"attestamp/internal/cli"
	cmp "attestamp/internal/core/compare/auditor"
)

// auditCmd runs the Auditor's part in one command: request an attestation document from its own TEE and verify it, compare it with the values I built and measured,
// register those values only if they match, then read the registration back and check it. The screen shows a sentence saying what is happening, not a step label.
//
// With only --tee-addr, the attested values are registered as they are (no check). With only --build-measurements, that file is registered as it is.
func auditCmd(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	teeAddr := fs.String("tee-addr", "", "my enclave's host:port — an attestation is requested from it first")
	vendorTee := fs.String("vendor-tee", "", "the Vendor's enclave host:port — also ask it, and say whether it runs the same value I built. it is registered either way: what I built, not what the Vendor runs")
	format := fs.String("format", "aws-nitro", "expected document format")
	timeout := fs.Duration("timeout", 30*time.Second, "RA request timeout")
	nonce := fs.String("nonce", "", "fixed nonce (e.g. for a test fixture). empty uses the one just sent")
	buildMeasurements := fs.String("build-measurements", "", "the value I built or measured myself (out/measurements.json). with --tee-addr the attested value must match it, or nothing is registered")
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	registry := fs.String("registry", "", "Registry contract address")
	signingKeyFile := fs.String("signing-key-file", "", "signing key file (hex): its signature authorises the registration, and the same key sends the transaction")
	platform := fs.String("platform", "aws-nitro", "measurement format")
	config := fs.String("config", "", "the Vendor's build-config.json (receive puts it in the clone root): service, version and build_env are taken from it, so --service/--version/--arch are optional and only override it, with a warning")
	service := fs.String("service", "", "service name. empty takes it from --config, else from an existing registration of the same measurements (anyone's)")
	version := fs.String("version", "", "version. empty takes it from --config, else from an existing registration, as above")
	arch := fs.String("arch", "", "CPU architecture the image was built for and measured on (amd64|arm64). default: build-config.json's with --config, else this machine's — give the instance's if --measurements came from elsewhere")
	expires := fs.Duration("expires", 90*24*time.Hour, "how long from now until the registration expires")
	trustedApprovers := fs.String("trusted-approvers", "", "trusted approver addresses, comma-separated. empty uses the registry's APPROVER")
	outDir := fs.String("out-dir", "", "directory for the intermediate files (document, parsed value). empty uses a temporary directory, removed at the end")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// With --config, service, version and arch come from it (a flag overrides it, with a one-line warning). Without it, service and version are
	// found in an existing registration below or asked for, and arch is this machine's.
	cfg, err := loadBuildConfig(*config)
	if err != nil {
		return err
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
		ask(teeAddr, "my enclave host:port (empty uses a measurements file)", "")
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

		t := cli.Start(fmt.Sprintf("Asking my enclave at %s for an attestation", *teeAddr))
		if err := raRequest([]string{"--tee-addr", *teeAddr, "--out-file", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
			t.Fail("no answer")
			return err
		}
		t.Done(fmt.Sprintf("%s document, %d bytes", *format, fileSize(docPath)))
		cli.Why("an attestation is a document the enclave hardware signs on the spot, stating which image is running; the fresh nonce inside proves it was made just now")

		t = cli.Start("Checking the attestation (signature chain to the platform root, nonce)")
		parseArgs := []string{"--doc", docPath, "--out-file", parsedPath}
		if *nonce != "" {
			parseArgs = append(parseArgs, "--nonce", *nonce)
		}
		if err := raParse(parseArgs); err != nil {
			t.Fail("invalid")
			return err
		}
		t.Done("genuine")
		cli.Why("the signature chains up to the platform vendor's root certificate, so this really came from an enclave; PCR0·1·2 below are the image it runs")
		printPCRs(parsedPath)
		if measPath == "" {
			measPath = parsedPath
		}
	} else {
		fmt.Printf("No enclave given — registering %s as it is, without checking it against a running enclave\n", measPath)
	}

	values, err := readMeasurements(measPath)
	if err != nil {
		return err
	}
	if parsedPath != "" && parsedPath != measPath {
		t := cli.Start(fmt.Sprintf("Comparing what the enclave attested with my own value (%s)", measPath))
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
			return errors.New("the enclave is not running what I built — not registering")
		}
		t.Done("identical — the enclave runs exactly what I built")
		cli.Why("this is the audit: the value I obtained myself and the value the running enclave attests to are the same, so the enclave is not running something else")
	}

	// Also ask what the Vendor is running right now. If it differs, say so, but the value registered is still the one I built:
	// what goes on the chain must be "what I verified", not "what the Vendor runs".
	vendorRunsSomethingElse := false
	if *vendorTee != "" {
		dir, err := os.MkdirTemp("", "att-auditor-vendor-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		vDoc, vParsed := filepath.Join(dir, "doc.cbor"), filepath.Join(dir, "parsed.json")
		t := cli.Start(fmt.Sprintf("Asking the Vendor's enclave at %s what it is running", *vendorTee))
		if err := raRequest([]string{"--tee-addr", *vendorTee, "--out-file", vDoc, "--format", *format, "--timeout", timeout.String()}); err != nil {
			t.Fail("no answer")
			return err
		}
		if err := raParse([]string{"--doc", vDoc, "--out-file", vParsed}); err != nil {
			t.Fail("invalid attestation")
			return err
		}
		vendorValues, err := readMeasurements(vParsed)
		if err != nil {
			return err
		}
		v := (cmp.Impl{}).Compare(vendorValues, values)
		if v.Pass {
			t.Done("same — the Vendor's enclave runs exactly what I built")
		} else {
			vendorRunsSomethingElse = true
			t.Fail("the Vendor's enclave runs something else")
			for _, r := range v.Reason {
				cli.Errorln("    ", r)
			}
			printPCRs(vParsed)
		}
		cli.Why("the Vendor claims to run this source; the only way to know is to ask its enclave and compare with what the source really builds to")
	}
	reader := lether.Reader{RPC: *rpc, RegistryAddr: common.HexToAddress(*registry)}

	if *service == "" || *version == "" {
		t := cli.Start("No service/version given — looking for an existing registration of the same value")
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
			t.Fail("none — enter them by hand")
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
	cli.Okln("PASS — audited and registered. Users can now verify against registry ", *registry)
	fmt.Printf("this registration is for %s only — put \"arch\": \"%s\" in measurement-record.json; a build for the other architecture measures differently and is registered on its own\n", *arch, *arch)
	if vendorRunsSomethingElse {
		cli.Errorln("but note: the Vendor's enclave at ", *vendorTee, " does NOT run this value. a User verifying that enclave against this registry will be told not to trust it")
	}
	return nil
}

// fileSize returns the size of the received document. Errors are swallowed since a 0 is enough if it fails.
func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// printPCRs prints the measurements in the parsed-values file, indented (PCR0, PCR1, PCR2).
func printPCRs(path string) {
	values, err := readMeasurements(path)
	if err != nil {
		return
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	for _, m := range values {
		fmt.Printf("    %-5s %x\n", m.Name, m.Value)
	}
}
