package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"

	lether "tb/internal/chain/lookup/user/ether"
	"tb/internal/cli"
	cmp "tb/internal/core/compare/user"
)

// verifyCmd — 흐름1의 User 구간(14~18)을 명령 하나로: T1 요청 → T2 파싱 → C2 조회 → 찾은 key 마다 M2 대조.
// 하나라도 PASS 면 그 엔클레이브를 신뢰할 근거가 있다는 뜻이라 exit 0, 하나도 없거나 다 FAIL 이면 exit 1.
func verifyCmd(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	tee := fs.String("tee", "", "TEE address host:port")
	format := fs.String("format", "aws-nitro", "expected document format")
	timeout := fs.Duration("timeout", 30*time.Second, "RA request timeout")
	nonce := fs.String("nonce", "", "fixed nonce (e.g. for a test fixture). empty uses the one just sent")
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	contract := fs.String("contract", "", "Registry contract address")
	trusted := fs.String("trusted", "", "trusted signer addresses, comma-separated. empty uses the contract's APPROVER")
	out := fs.String("out", "", "directory for the intermediate files (document, measurements). empty uses a temporary directory, removed at the end")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 옵션으로 안 준 값은 lookup 처럼 차례로 묻는다.
	ask(tee, "TEE address host:port", "")
	ask(rpc, "RPC URL", "https://ethereum-rpc.publicnode.com")
	ask(contract, "Registry contract address (0x…)", "")
	if *tee == "" || *rpc == "" || !common.IsHexAddress(*contract) {
		return errors.New("--tee, --rpc and --contract are required")
	}

	dir := *out
	if dir == "" {
		tmp, err := os.MkdirTemp("", "tb-user-verify-*")
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

	fmt.Println("== 1/4 T1 RA request")
	if err := raRequest([]string{"--tee", *tee, "--out", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
		return err
	}

	fmt.Println("\n== 2/4 T2 RA parse")
	parseArgs := []string{"--doc", docPath, "--out", measPath}
	if *nonce != "" {
		parseArgs = append(parseArgs, "--nonce", *nonce)
	}
	if err := raParse(parseArgs); err != nil {
		return err
	}
	values, err := readMeasurements(measPath)
	if err != nil {
		return err
	}

	fmt.Println("\n== 3/4 C2 chain lookup")
	keys, err := lookup([]string{"--rpc", *rpc, "--contract", *contract, "--measurements", measPath})
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return errors.New("nobody has registered these measurements on the chain — no basis for trust")
	}

	fmt.Println("\n== 4/4 M2 compare (chain value vs parsed value, per key)")
	reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(*contract)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	trustedList, err := trustedSigners(ctx, reader, *trusted)
	if err != nil {
		return err
	}
	anyPass := false
	for _, k := range keys {
		reg, err := reader.Lookup(ctx, k)
		if err != nil {
			return err
		}
		v := (cmp.Impl{}).Check(values, reg, time.Now().Unix(), trustedList)
		if v.Pass {
			fmt.Printf("  %s  PASS\n", k)
			anyPass = true
			continue
		}
		cli.Errorf("  %s  FAIL", k)
		for _, r := range v.Reason {
			cli.Errorln("    - ", r)
		}
	}
	if !anyPass {
		fmt.Fprintln(os.Stderr)
		cli.Errorln("no registration matches this attestation")
		os.Exit(1)
	}
	fmt.Println("\ntrusted — at least one registration matches this attestation")
	return nil
}
