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

	lether "tb/internal/chain/lookup/user/ether"
	"tb/internal/cli"
	cmp "tb/internal/core/compare/user"
)

// verifyCmd — 흐름1의 User 구간(14~18)을 명령 하나로: 엔클레이브에 attestation 을 받아(T1·T2) → 그 값으로 등록부에서 등록을
// 찾고(C2) → 찾은 것마다 값·만료·서명자를 맞댄다(M2). 화면에는 단계 이름표 대신 지금 무엇을 하는지 문장으로 찍는다.
//
// --contract 는 여러 번 줄 수 있다 (흐름 2 의 21~23: Vendor 의 등록부와 Auditor 의 등록부를 나란히). 등록부 하나 = 승인자 하나라,
// 등록부마다 "이 엔클레이브가 돌리는 값을 그 승인자가 등록했나"를 따로 보여 주고, **준 등록부 전부**에 맞는 등록이 있어야 신뢰한다.
// 하나만 주면 예전과 같다: 맞는 등록이 하나라도 있으면 신뢰.
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
	// 옵션으로 안 준 값은 lookup 처럼 차례로 묻는다.
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
	vouched := 0 // 맞는 등록이 하나 이상 있는 등록부 수
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
