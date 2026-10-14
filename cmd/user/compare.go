package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	lether "attestamp/internal/chain/lookup/user/ether"
	"attestamp/internal/cli"
	cmp "attestamp/internal/core/compare/user"
)

// compareCmd: with --expected, compares two sets of measurements directly (Compare). With --record-key, reads the
// registration from the chain and also checks expiry and the approver (Check).
func compareCmd(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	attested := fs.String("attested", "", "the attested measurements: a measurements.json file, or NAME=hex,NAME=hex")
	expected := fs.String("expected", "", "the expected measurements, same shape — required unless --record-key is used")
	rpc := fs.String("rpc", "", "chain mode: Ethereum RPC URL")
	registry := fs.String("registry", "", "chain mode: Registry contract address")
	recordKey := fs.String("record-key", "", "chain mode: record key (the one register printed)")
	trustedApprovers := fs.String("trusted-approvers", "", "chain mode: trusted approver addresses, comma-separated. empty uses the registry's APPROVER")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *attested == "" {
		return errors.New("--attested is required")
	}
	if err := checkMode(*expected, *rpc, *registry, *recordKey, *trustedApprovers); err != nil {
		return err
	}
	attestedValues, err := readMeasurements(*attested)
	if err != nil {
		return err
	}

	if *recordKey != "" {
		if *rpc == "" || !common.IsHexAddress(*registry) {
			return errors.New("chain mode needs --attested, --registry, --rpc and --record-key together")
		}
		reader := lether.Reader{RPC: *rpc, RegistryAddr: common.HexToAddress(*registry)}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		reg, err := reader.Lookup(ctx, *recordKey)
		if err != nil {
			return err
		}
		approvers, err := resolveTrustedApprovers(ctx, reader, *trustedApprovers)
		if err != nil {
			return err
		}
		return printVerdict((cmp.Impl{}).Check(attestedValues, reg, cmp.Clock(reg), approvers))
	}

	if *expected == "" {
		return errors.New("--expected or (--registry and --record-key) is required")
	}
	expectedValues, err := readMeasurements(*expected)
	if err != nil {
		return err
	}
	return printVerdict((cmp.Impl{}).Compare(attestedValues, expectedValues))
}

// checkMode accepts exactly one of --expected (plain mode) and --record-key (chain mode) and reports an error when both are given.
// --rpc, --registry and --trusted-approvers belong to chain mode, so each is rejected without --record-key.
func checkMode(expected, rpc, registry, recordKey, trustedApprovers string) error {
	chainFlags := rpc != "" || registry != "" || recordKey != "" || trustedApprovers != ""
	switch {
	case expected != "" && recordKey != "":
		return errors.New("--expected (plain compare) and --record-key (chain lookup) cannot be given together — pick one")
	case expected != "" && chainFlags:
		return errors.New("--expected was given along with --rpc/--registry/--trusted-approvers — without --record-key this is not chain mode, drop them")
	case recordKey == "" && (rpc != "" || registry != ""):
		return errors.New("--rpc or --registry needs --record-key as well to be chain mode")
	case recordKey == "" && trustedApprovers != "":
		return errors.New("--trusted-approvers is unused without --record-key (chain mode only)")
	}
	return nil
}

// resolveTrustedApprovers: the --trusted-approvers list when given, else the registry's own approver.
func resolveTrustedApprovers(ctx context.Context, reader lether.Reader, flagValue string) ([]string, error) {
	if flagValue != "" {
		return strings.Split(flagValue, ","), nil
	}
	return reader.TrustedApprovers(ctx)
}

// printVerdict prints the verdict and exits 1 on failure (so scripts can read success from the exit code).
func printVerdict(v cmp.Verdict) error {
	if v.Pass {
		cli.Okln("PASS")
		return nil
	}
	cli.Errorln("FAIL")
	for _, r := range v.Reason {
		cli.Errorln("  - ", r)
	}
	os.Exit(1)
	return nil
}
