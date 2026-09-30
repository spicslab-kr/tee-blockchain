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

	lether "tb/internal/chain/lookup/auditor/ether"
	"tb/internal/cli"
	cmp "tb/internal/core/compare/auditor"
)

// auditCmd — 흐름 1 의 Auditor 구간을 명령 하나로: 자기 TEE 에 attestation 을 받아(T1·T2) 내가 빌드·측정한 값과 맞대고(M2),
// 같을 때만 그 값을 등록하고(C1), 읽어서 자기확인(C2·M2). 화면에는 단계 이름표 대신 지금 무엇을 하는지 문장으로 찍는다.
//
// --tee 만 주면 증명값을 그대로 등록한다(검사 없음). --measurements 만 주면 그 파일을 그대로 등록한다.
func auditCmd(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	tee := fs.String("tee", "", "my enclave's host:port — an attestation is requested from it first")
	vendorTee := fs.String("vendor-tee", "", "the Vendor's enclave host:port — also ask it, and say whether it runs the same value I built (flow 2 step 15). it is registered either way: what I built, not what the Vendor runs")
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
	arch := fs.String("arch", defaultArch(), "CPU architecture the image was built for and measured on (amd64|arm64). default: this machine's — give the instance's if --measurements came from elsewhere")
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
	if err := checkArch(*arch); err != nil {
		return err
	}
	quiet = true // 아래 단계들이 raRequest · raParse · register 를 부르는데, 그들의 출력 대신 여기서 한 줄씩 찍는다

	measPath := *measurements
	parsedPath := ""
	if *tee != "" {
		dir := *out
		if dir == "" {
			tmp, err := os.MkdirTemp("", "tb-auditor-audit-*")
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
		cli.Why("an attestation is a document the enclave hardware signs on the spot, stating which image is running; the fresh nonce inside proves it was made just now")

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

	// 흐름 2 의 8·9·15: Vendor 가 지금 띄워 둔 것도 물어본다. 다르면 그 사실을 찍되, 등록하는 값은 내가 빌드한 값 그대로다 —
	// 체인에 올라가는 건 "Vendor 가 돌리는 것"이 아니라 "내가 검증한 것"이어야 하니까.
	vendorRunsSomethingElse := false
	if *vendorTee != "" {
		dir, err := os.MkdirTemp("", "tb-auditor-vendor-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		vDoc, vParsed := filepath.Join(dir, "doc.cbor"), filepath.Join(dir, "parsed.json")
		t := cli.Start(fmt.Sprintf("Asking the Vendor's enclave at %s what it is running", *vendorTee))
		if err := raRequest([]string{"--tee", *vendorTee, "--out", vDoc, "--format", *format, "--timeout", timeout.String()}); err != nil {
			t.Fail("no answer")
			return err
		}
		if err := raParse([]string{"--doc", vDoc, "--out", vParsed}); err != nil {
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

	t := cli.Start(fmt.Sprintf("Registering %s %s (%s) on the chain, signed with my key", *service, *version, *arch))
	key, err := register([]string{
		"--rpc", *rpc, "--contract", *contract, "--key-file", *keyFile,
		"--measurements", measPath, "--platform", *platform, "--arch", *arch,
		"--service", *service, "--version", *version, "--expires", expires.String(),
	})
	if err != nil {
		t.Fail("rejected")
		return err
	}
	t.Done("registered")
	cli.Why("the value now sits on the public registry under my signature; anyone can look it up by measurement alone, without asking me")
	fmt.Printf("    key     %s\n    tx      %s\n    block   %d\n    expires %s\n    signer  %s\n    build env  %s\n",
		lastRegistered.Key, lastRegistered.Tx, lastRegistered.Block, lastRegistered.Expires, lastRegistered.Signer, lastRegistered.BuildEnv)

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
	cli.Why("the same check a User will run: read the registration back and make sure it matches, is not expired, and was signed by the approver this registry trusts")
	cli.Okln("PASS — audited and registered. Users can now verify against contract ", *contract)
	fmt.Printf("this registration is for %s only — put \"arch\": \"%s\" in measurement-record.json; a build for the other architecture measures differently and is registered on its own\n", *arch, *arch)
	if vendorRunsSomethingElse {
		cli.Errorln("but note: the Vendor's enclave at ", *vendorTee, " does NOT run this value. a User verifying that enclave against this registry will be told not to trust it")
	}
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
