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

	lether "tb/internal/chain/lookup/auditor/ether"
	"tb/internal/cli"
	cmp "tb/internal/core/compare/auditor"
)

// runCmd — register 하고 바로 그 자리에서 self-check 까지 명령 하나로 (C1 → C2 → M2).
// --tee 를 주면 그 전에 T1 요청 → T2 파싱까지 해서 측정값을 직접 얻는다(흐름1의 11~13, 흐름2의 2~3에
// 해당). 이미 측정값 파일이 있으면 --measurements 로 바로 등록만 해도 된다.
//
// --tee 와 --measurements 를 같이 주면 검사가 된다(흐름1의 3~6): TEE 가 증명한 값을 --measurements(내가
// 소스로 빌드한 값)와 먼저 맞대고, 같을 때만 그 빌드값을 등록한다. 다르면 등록하지 않고 실패로 끝난다 —
// 그래야 "지금 도는 것"이 아니라 "내가 검증한 소스"가 체인에 올라간다.
func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	tee := fs.String("tee", "", "TEE address host:port — if given, RA is requested from it first to obtain the measurements")
	format := fs.String("format", "aws-nitro", "expected document format")
	timeout := fs.Duration("timeout", 30*time.Second, "RA request timeout")
	nonce := fs.String("nonce", "", "fixed nonce (e.g. for a test fixture). empty uses the one just sent")
	measurements := fs.String("measurements", "", "an existing measurements file (e.g. build's out/measurements.json). without --tee it is registered as is; with --tee it is first checked against the TEE-attested value and registered only if they match")
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	contract := fs.String("contract", "", "Registry contract address")
	keyFile := fs.String("key-file", "", "private key file (hex) that signs and sends the transaction")
	platform := fs.String("platform", "aws-nitro", "measurement format")
	service := fs.String("service", "", "service name. empty takes it from an existing registration of the same measurements (anyone's)")
	version := fs.String("version", "", "version. empty takes it from an existing registration, as above")
	expires := fs.Duration("expires", 90*24*time.Hour, "how long from now until the registration expires")
	trusted := fs.String("trusted", "", "trusted signer addresses, comma-separated. empty uses the contract's APPROVER")
	out := fs.String("out", "", "directory for the T1·T2 intermediate files. empty uses a temporary directory, removed at the end")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 옵션으로 안 준 값은 register 처럼 차례로 묻는다 — plain 에서 전부 플래그로 줄 필요는 없다.
	askMainnetRPC(rpc)
	ask(contract, "Registry contract address (0x…)", "")
	ask(keyFile, "my private key file", "")
	if *tee == "" && *measurements == "" {
		ask(tee, "TEE address host:port (empty uses a measurements file)", "")
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

	measPath := *measurements
	parsedPath := ""
	if *tee != "" {
		dir := *out
		if dir == "" {
			tmp, err := os.MkdirTemp("", "tb-auditor-run-*")
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

		fmt.Println("== 1/3 T1 RA request")
		if err := raRequest([]string{"--tee", *tee, "--out", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
			return err
		}
		fmt.Println("\n== 2/3 T2 RA parse")
		parseArgs := []string{"--doc", docPath, "--out", parsedPath}
		if *nonce != "" {
			parseArgs = append(parseArgs, "--nonce", *nonce)
		}
		if err := raParse(parseArgs); err != nil {
			return err
		}
		if measPath == "" {
			measPath = parsedPath
		}
	} else {
		fmt.Println("== 1/3 (skipped — using --measurements as is)")
		fmt.Println("== 2/3 (skipped)")
	}

	values, err := readMeasurements(measPath)
	if err != nil {
		return err
	}
	if parsedPath != "" && parsedPath != measPath {
		fmt.Println("\n== audit: TEE-attested value vs --measurements (my build value)")
		parsed, err := readMeasurements(parsedPath)
		if err != nil {
			return err
		}
		v := (cmp.Impl{}).Compare(parsed, values)
		if !v.Pass {
			cli.Errorln("FAIL")
			for _, r := range v.Reason {
				cli.Errorln("  - ", r)
			}
			return errors.New("the TEE-attested value differs from --measurements — not registering")
		}
		fmt.Println("PASS")
	}
	reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(*contract)}

	if *service == "" || *version == "" {
		fmt.Println("\n== service/version empty — taking them from an existing registration of the same measurements")
		ictx, icancel := context.WithTimeout(context.Background(), time.Minute)
		keys, err := reader.FindAddress(ictx, values)
		if err != nil {
			icancel()
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
			fmt.Printf("   %s %s (taken from key %s)\n", existing.Meta.Service, existing.Meta.Version, k)
			resolved = true
			break
		}
		icancel()
		if !resolved {
			fmt.Println("   no existing registration of the same measurements — enter them by hand")
		}
		ask(service, "service name", "")
		ask(version, "version", "")
		if *service == "" || *version == "" {
			return errors.New("--service and --version are required")
		}
	}

	fmt.Println("\n== C1 register, then M2 self-check")
	key, err := register([]string{
		"--rpc", *rpc, "--contract", *contract, "--key-file", *keyFile,
		"--measurements", measPath, "--platform", *platform,
		"--service", *service, "--version", *version, "--expires", expires.String(),
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	reg, err := reader.Lookup(ctx, key)
	if err != nil {
		return err
	}
	trustedList, err := trustedSigners(ctx, reader, *trusted)
	if err != nil {
		return err
	}
	return printVerdict((cmp.Impl{}).Check(values, reg, time.Now().Unix(), trustedList))
}
