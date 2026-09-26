package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	lether "tb/internal/chain/lookup/auditor/ether"
	"tb/internal/cli"
	cmp "tb/internal/core/compare/auditor"
)

// compareCmd — M2. --b 가 있으면 측정값 두 묶음을 그대로 맞댄다(Compare). --key 가 있으면 체인에서
// 등록 정보를 읽어 만료 · 신뢰 서명자까지 같이 본다(Check).
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
		return printVerdict((cmp.Impl{}).Check(va, reg, time.Now().Unix(), trustedList))
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

// checkMode — --b(평문 모드)와 --key(체인 모드)는 딱 하나만 받는다. 실수로 둘 다 주면 어느 하나를 조용히
// 무시하고 넘어가지 않고 바로 에러로 알린다. --trusted 는 체인 모드에서만 뜻이 있다 — --key 없이 주면
// 그냥 버려지므로 이것도 미리 막는다.
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

// printVerdict — 판정을 찍고, 실패면 exit 1 로 끝낸다(스크립트에서 성공 여부를 종료코드로 보게).
func printVerdict(v cmp.Verdict) error {
	if v.Pass {
		fmt.Println("PASS")
		return nil
	}
	cli.Errorln("FAIL")
	for _, r := range v.Reason {
		cli.Errorln("  - ", r)
	}
	os.Exit(1)
	return nil
}
