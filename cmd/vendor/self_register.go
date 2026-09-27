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

	lether "tb/internal/chain/lookup/Vendor/ether"
	"tb/internal/cli"
	cmp "tb/internal/core/compare/Vendor"
)

// selfRegisterCmd — 흐름 2, Auditor 없이 Vendor 가 자기 엔클레이브를 직접 등록: 자기 TEE 에 attestation 을 받아(T1·T2),
// --measurements 가 있으면 내 빌드값과 맞대고(M2), 등록하고(C1), 읽어서 자기확인(C2·M2). audit 와 같은 몸통이다.
func selfRegisterCmd(args []string) error {
	fs := flag.NewFlagSet("self-register", flag.ContinueOnError)
	tee := fs.String("tee", "", "my enclave's host:port — an attestation is requested from it first")
	format := fs.String("format", "aws-nitro", "expected document format")
	timeout := fs.Duration("timeout", 30*time.Second, "RA request timeout")
	nonce := fs.String("nonce", "", "fixed nonce (e.g. for a test fixture). empty uses the one just sent")
	measurements := fs.String("measurements", "", "the value I built or measured myself (out/measurements.json). with --tee the attested value must match it, or nothing is registered")
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	contract := fs.String("contract", "", "Registry contract address")
	keyFile := fs.String("key-file", "", "private key file (hex) that signs and sends the transaction")
	platform := fs.String("platform", "aws-nitro", "measurement format")
	service := fs.String("service", "", "service name. empty takes it from an existing registration of the same measurements (anyone's)")
	version := fs.String("version", "", "version. empty takes it from an existing registration, as above")
	expires := fs.Duration("expires", 90*24*time.Hour, "how long from now until the registration expires")
	trusted := fs.String("trusted", "", "trusted signer addresses, comma-separated. empty uses the contract's APPROVER")
	out := fs.String("out", "", "directory for the intermediate files (document, parsed value). empty uses a temporary directory, removed at the end")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 옵션으로 안 준 값은 register 처럼 차례로 묻는다 — plain 에서 전부 플래그로 줄 필요는 없다.
	askMainnetRPC(rpc)
	ask(contract, "Registry contract address (0x…)", "")
	ask(keyFile, "my private key file", "")
	if *tee == "" && *measurements == "" {
		ask(tee, "my enclave host:port (empty uses a measurements file)", "")
	}
	if *tee == "" && *measurements == "" {
		ask(measurements, "measurements file", "out/measurements.json")
	}
	if *rpc == "" || !common.IsHexAddress(*contract) || *keyFile == "" {
		return errors.New("--rpc, --contract and --key-file are required")
	}
	if *tee == "" && *measurements == "" {
		return errors.New("--tee or --measurements is required")
	}
	quiet = true // 아래 단계들이 raRequest · raParse · register 를 부르는데, 그들의 출력 대신 여기서 한 줄씩 찍는다

	measPath := *measurements
	parsedPath := ""
	if *tee != "" {
		dir := *out
		if dir == "" {
			tmp, err := os.MkdirTemp("", "tb-vendor-self-register-*")
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

		t := cli.Start(fmt.Sprintf("Asking my enclave at %s for an attestation", *tee))
		if err := raRequest([]string{"--tee", *tee, "--out", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
			t.Fail("no answer")
			return err
		}
		t.Done(fmt.Sprintf("%s document, %d bytes", *format, fileSize(docPath)))

		t = cli.Start("Checking the attestation (signature chain to the platform root, nonce)")
		parseArgs := []string{"--doc", docPath, "--out", parsedPath}
		if *nonce != "" {
			parseArgs = append(parseArgs, "--nonce", *nonce)
		}
		if err := raParse(parseArgs); err != nil {
			t.Fail("invalid")
			return err
		}
		t.Done("genuine")
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
	}
	reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(*contract)}

	if *service == "" || *version == "" {
		t := cli.Start("No service/version given — looking for an existing registration of the same value")
		ictx, icancel := context.WithTimeout(context.Background(), time.Minute)
		keys, err := reader.FindAddress(ictx, values)
		if err != nil {
			icancel()
			t.Fail("chain error")
			return err
		}
		resolved := false
		for _, k := range keys {
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

	t := cli.Start(fmt.Sprintf("Registering %s %s on the chain, signed with my key", *service, *version))
	key, err := register([]string{
		"--rpc", *rpc, "--contract", *contract, "--key-file", *keyFile,
		"--measurements", measPath, "--platform", *platform,
		"--service", *service, "--version", *version, "--expires", expires.String(),
	})
	if err != nil {
		t.Fail("rejected")
		return err
	}
	t.Done("registered")
	fmt.Printf("    key     %s\n    tx      %s\n    block   %d\n    expires %s\n    signer  %s\n",
		lastRegistered.Key, lastRegistered.Tx, lastRegistered.Block, lastRegistered.Expires, lastRegistered.Signer)

	t = cli.Start("Reading the registration back from the chain and checking it")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	reg, err := reader.Lookup(ctx, key)
	if err != nil {
		t.Fail("cannot read")
		return err
	}
	trustedList, err := trustedSigners(ctx, reader, *trusted)
	if err != nil {
		t.Fail("cannot read the approver")
		return err
	}
	v := (cmp.Impl{}).Check(values, reg, time.Now().Unix(), trustedList)
	if !v.Pass {
		t.Fail("something is off")
		for _, r := range v.Reason {
			cli.Errorln("    ", r)
		}
		cli.Errorln("FAIL")
		os.Exit(1)
	}
	t.Done("value, expiry and signer all check out")
	cli.Okln("PASS — registered by myself (no Auditor). Users can verify against contract ", *contract)
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
