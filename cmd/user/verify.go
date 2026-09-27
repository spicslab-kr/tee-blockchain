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

	lether "tb/internal/chain/lookup/user/ether"
	"tb/internal/cli"
	cmp "tb/internal/core/compare/user"
)

// verifyCmd — 흐름1의 User 구간(14~18)을 명령 하나로: 엔클레이브에 attestation 을 받아(T1·T2) → 그 값으로 체인에서 등록을 찾고(C2)
// → 찾은 것마다 값·만료·서명자를 맞댄다(M2). 하나라도 맞으면 신뢰할 근거가 있다 → exit 0. 없으면 exit 1.
// 화면에는 단계 이름표 대신 지금 무엇을 하는지 문장으로 찍는다.
func verifyCmd(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	tee := fs.String("tee", "", "the enclave's host:port")
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
	ask(tee, "enclave host:port", "")
	ask(rpc, "RPC URL", "https://ethereum-rpc.publicnode.com")
	ask(contract, "Registry contract address (0x…)", "")
	if *tee == "" || *rpc == "" || !common.IsHexAddress(*contract) {
		return errors.New("--tee, --rpc and --contract are required")
	}
	quiet = true

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

	t := cli.Start(fmt.Sprintf("Asking the enclave at %s for an attestation", *tee))
	if err := raRequest([]string{"--tee", *tee, "--out", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
		t.Fail("no answer")
		return err
	}
	t.Done(fmt.Sprintf("%s document, %d bytes", *format, fileSize(docPath)))

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
	printPCRs(measPath)
	values, err := readMeasurements(measPath)
	if err != nil {
		return err
	}

	t = cli.Start("Looking up who registered these measurements on the chain")
	keys, err := lookup([]string{"--rpc", *rpc, "--contract", *contract, "--measurements", measPath})
	if err != nil {
		t.Fail("chain error")
		return err
	}
	if len(keys) == 0 {
		t.Fail("nobody")
		return errors.New("nobody has registered these measurements on the chain — no basis for trust")
	}
	t.Done(fmt.Sprintf("%d registration(s)", len(keys)))

	reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(*contract)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	trustedList, err := trustedSigners(ctx, reader, *trusted)
	if err != nil {
		return err
	}
	fmt.Println("Checking each registration against the attestation (value, expiry, signer):")
	anyPass := false
	var goodSigner string
	for _, k := range keys {
		reg, err := reader.Lookup(ctx, k)
		if err != nil {
			return err
		}
		v := (cmp.Impl{}).Check(values, reg, time.Now().Unix(), trustedList)
		if v.Pass {
			fmt.Printf("    %s  %s  %s %s by %s, expires %s\n", k, cli.Green("✓ matches"), reg.Meta.Service, reg.Meta.Version, reg.Signer,
				time.Unix(reg.Expires, 0).UTC().Format(time.RFC3339))
			anyPass = true
			goodSigner = reg.Signer
			continue
		}
		fmt.Printf("    %s  %s\n", k, cli.Red("✗ does not match"))
		for _, r := range v.Reason {
			cli.Errorln("        ", r)
		}
	}
	if !anyPass {
		cli.Errorln("not trusted — no registration matches this attestation")
		os.Exit(1)
	}
	cli.Okln("trusted — this enclave runs exactly what ", goodSigner, " registered")
	return nil
}

// fileSize — 받은 문서 크기. 실패해도 0 만 찍히면 되니 오류는 삼킨다.
func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// printPCRs — 파싱값 파일의 측정값을 들여쓰기해 찍는다 (PCR0 · PCR1 · PCR2).
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
