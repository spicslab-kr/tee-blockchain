package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	lether "tb/internal/chain/lookup/user/ether"
	"tb/internal/cli"
	cmp "tb/internal/core/compare/user"
)

// compareCmd: with --b, compares two sets of measurements directly (Compare). With --key, reads the
// registration from the chain and also checks expiry and the trusted signer (Check).
func compareCmd(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	a := fs.String("a", "", "measurements A: a measurements.json file, or NAME=hex,NAME=hex")
	b := fs.String("b", "", "measurements B — required unless --key is used")
	rpc := fs.String("rpc", "", "chain mode: Ethereum RPC URL")
	contract := fs.String("contract", "", "chain mode: Registry contract address")
	key := fs.String("key", "", "chain mode: lookup key (the key from register)")
	trusted := fs.String("trusted", "", "chain mode: trusted signer addresses, comma-separated. empty uses the contract's APPROVER")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *a == "" {
		return errors.New("--a is required")
	}
	if err := checkMode(*b, *rpc, *contract, *key, *trusted); err != nil {
		return err
	}
	va, err := readMeasurements(*a)
	if err != nil {
		return err
	}

	if *key != "" {
		if *rpc == "" || !common.IsHexAddress(*contract) {
			return errors.New("chain mode needs --rpc, --contract and --key together")
		}
		reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(*contract)}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		reg, err := reader.Lookup(ctx, *key)
		if err != nil {
			return err
		}
		trustedList, err := trustedSigners(ctx, reader, *trusted)
		if err != nil {
			return err
		}
		return printVerdict((cmp.Impl{}).Check(va, reg, cmp.Clock(reg), trustedList))
	}

	if *b == "" {
		return errors.New("--b or (--contract and --key) is required")
	}
	vb, err := readMeasurements(*b)
	if err != nil {
		return err
	}
	return printVerdict((cmp.Impl{}).Compare(va, vb))
}

// checkMode accepts exactly one of --b (plain mode) and --key (chain mode). Giving both is reported as an error
// rather than silently ignoring one. --trusted only means something in chain mode; without --key it would
// just be dropped, so that is rejected up front too.
func checkMode(b, rpc, contract, key, trusted string) error {
	chainFlags := rpc != "" || contract != "" || key != "" || trusted != ""
	switch {
	case b != "" && key != "":
		return errors.New("--b (plain compare) and --key (chain lookup) cannot be given together — pick one")
	case b != "" && chainFlags:
		return errors.New("--b was given along with --rpc/--contract/--trusted — without --key this is not chain mode, drop them")
	case key == "" && (rpc != "" || contract != ""):
		return errors.New("--rpc or --contract needs --key as well to be chain mode")
	case key == "" && trusted != "":
		return errors.New("--trusted is unused without --key (chain mode only)")
	}
	return nil
}

func trustedSigners(ctx context.Context, reader lether.Reader, flag string) ([]string, error) {
	if flag != "" {
		return strings.Split(flag, ","), nil
	}
	return reader.TrustedSigners(ctx)
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
