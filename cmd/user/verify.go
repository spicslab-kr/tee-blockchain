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

	lether "attestamp/internal/chain/lookup/user/ether"
	"attestamp/internal/cli"
	cmp "attestamp/internal/core/compare/user"
)

// verifyCmd runs the User's part in one command: request an attestation document from the enclave and verify it, use its values to find the registration in the registry,
// then compare value, expiry and signer for each one found. The screen shows a sentence saying what is happening, not a step label.
//
// --contract may be given several times (for example the Vendor's registry and the Auditor's side by side). One registry = one approver, so
// for each registry it separately shows whether that approver registered what this enclave runs, and trust requires a matching registration in **every registry given**.
// With just one, it is as before: trusted if at least one registration matches.
func verifyCmd(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	tee := fs.String("tee", "", "the enclave's host:port")
	format := fs.String("format", "aws-nitro", "expected document format")
	timeout := fs.Duration("timeout", 30*time.Second, "RA request timeout")
	nonce := fs.String("nonce", "", "fixed nonce (e.g. for a test fixture). empty uses the one just sent")
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	var contracts []string
	fs.Func("contract", "Registry contract address. repeat it to check several registries (e.g. the Vendor's and the Auditor's); then every one of them must vouch for this enclave", func(s string) error {
		for _, c := range strings.Split(s, ",") {
			if c = strings.TrimSpace(c); c != "" {
				contracts = append(contracts, c)
			}
		}
		return nil
	})
	trusted := fs.String("trusted", "", "trusted signer addresses, comma-separated. empty uses each contract's APPROVER")
	out := fs.String("out", "", "directory for the intermediate files (document, measurements). empty uses a temporary directory, removed at the end")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Values not given as options are asked for in turn, as lookup does.
	ask(tee, "enclave host:port", "")
	ask(rpc, "RPC URL", "https://ethereum-rpc.publicnode.com")
	if len(contracts) == 0 {
		var c string
		ask(&c, "Registry contract address (0x…)", "")
		if c != "" {
			contracts = []string{c}
		}
	}
	if *tee == "" || *rpc == "" || len(contracts) == 0 {
		return errors.New("--tee, --rpc and --contract are required")
	}
	for _, c := range contracts {
		if !common.IsHexAddress(c) {
			return fmt.Errorf("--contract %s: not an address", c)
		}
	}
	quiet = true

	dir := *out
	if dir == "" {
		tmp, err := os.MkdirTemp("", "att-user-verify-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		dir = tmp
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	docPath := filepath.Join(dir, "doc.cbor")
	measPath := filepath.Join(dir, "measurements.json")

	t := cli.Start(fmt.Sprintf("Asking the enclave at %s for an attestation", *tee))
	if err := raRequest([]string{"--tee", *tee, "--out", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
		t.Fail("no answer")
		return err
	}
	t.Done(fmt.Sprintf("%s document, %d bytes", *format, fileSize(docPath)))
	cli.Why("an attestation is a document the enclave hardware signs on the spot, stating which image is running; the fresh nonce inside proves it was made just now")

	t = cli.Start("Checking the attestation (signature chain to the platform root, nonce)")
	parseArgs := []string{"--doc", docPath, "--out", measPath}
	if *nonce != "" {
		parseArgs = append(parseArgs, "--nonce", *nonce)
	}
	if err := raParse(parseArgs); err != nil {
		t.Fail("invalid")
		return err
	}
	t.Done("genuine")
	cli.Why("the signature chains up to the platform vendor's root certificate, so this really came from an enclave; PCR0·1·2 below are the image it runs")
	printPCRs(measPath)
	values, err := readMeasurements(measPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	several := len(contracts) > 1
	vouched := 0 // number of registries with at least one matching registration
	var signers []string
	for _, c := range contracts {
		reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(c)}
		trustedList, err := trustedSigners(ctx, reader, *trusted)
		if err != nil {
			return err
		}
		label := "the registry at " + c
		if several {
			label = fmt.Sprintf("registry %s (approver %s)", c, strings.Join(trustedList, ","))
		}
		t = cli.Start("Looking up who registered what this enclave runs, in " + label)
		keys, err := lookup([]string{"--rpc", *rpc, "--contract", c, "--measurements", measPath})
		if err != nil {
			t.Fail("chain error")
			return err
		}
		if len(keys) == 0 {
			t.Fail("nobody — this registry's approver never registered the value this enclave runs")
			continue
		}
		t.Done(fmt.Sprintf("%d registration(s)", len(keys)))
		if !several {
			cli.Why("the registry is searched by the measurement itself — no need to know who registered it or to trust the Vendor's word")
		}
		fmt.Println("    checking each against the attestation (value, expiry, signer):")
		okHere := false
		for _, k := range keys {
			reg, err := reader.Lookup(ctx, k)
			if err != nil {
				return err
			}
			v := (cmp.Impl{}).Check(values, reg, cmp.Clock(reg), trustedList)
			if v.Pass {
				fmt.Printf("    %s  %s  %s %s (%s) by %s, expires %s\n", k, cli.Green("✓ matches"), reg.Meta.Service, reg.Meta.Version, reg.Meta.Arch, reg.Signer,
					time.Unix(reg.Expires, 0).UTC().Format(time.RFC3339))
				okHere = true
				signers = append(signers, reg.Signer)
				continue
			}
			fmt.Printf("    %s  %s\n", k, cli.Red("✗ does not match"))
			for _, r := range v.Reason {
				cli.Errorln("        ", r)
			}
		}
		if okHere {
			vouched++
		}
	}
	if several {
		cli.Why("with several registries, every one of them has to vouch for what this enclave runs — if the Vendor's registry says yes but the Auditor's says no, the Auditor never approved what is running")
	} else {
		cli.Why("a registration counts only if its value equals what the enclave attested, it has not expired, and it was signed by the approver this registry trusts")
	}
	if vouched == 0 {
		return errors.New("nobody has registered these measurements on the chain — no basis for trust")
	}
	if vouched < len(contracts) {
		cli.Errorln(fmt.Sprintf("not trusted — only %d of %d registries vouch for what this enclave runs; the others never approved it", vouched, len(contracts)))
		os.Exit(1)
	}
	if several {
		cli.Okln("trusted — every registry given vouches for what this enclave runs (", strings.Join(uniq(signers), ", "), ")")
	} else {
		cli.Okln("trusted — this enclave runs exactly what ", strings.Join(uniq(signers), ", "), " registered")
	}
	return nil
}

func uniq(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
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
