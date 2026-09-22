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

	lether "tb/internal/chain/lookup/Vendor/ether"
	cmp "tb/internal/core/compare/Vendor"
)

// runCmd — register 하고 바로 그 자리에서 self-check 까지 명령 하나로 (C1 → C2 → M2).
// --tee 를 주면 그 전에 T1 요청 → T2 파싱까지 해서 측정값을 직접 얻는다(흐름1의 11~13, 흐름2의 2~3에
// 해당). 이미 측정값 파일이 있으면 --measurements 로 바로 등록만 해도 된다.
func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	tee := fs.String("tee", "", "TEE 주소 host:port — 있으면 여기서 먼저 RA 를 요청해 측정값을 얻는다")
	format := fs.String("format", "aws-nitro", "기대하는 문서 형식")
	timeout := fs.Duration("timeout", 30*time.Second, "RA 요청 타임아웃")
	nonce := fs.String("nonce", "", "고정된 nonce(테스트용 fixture 등). 비우면 방금 보낸 것을 쓴다")
	measurements := fs.String("measurements", "", "이미 있는 측정값 파일. --tee 대신 쓴다")
	rpc := fs.String("rpc", "", "이더리움 RPC 주소")
	contract := fs.String("contract", "", "Registry 컨트랙트 주소")
	keyFile := fs.String("key-file", "", "서명하고 트랜잭션을 보낼 개인키 파일 (hex)")
	platform := fs.String("platform", "aws-nitro", "측정값 형식")
	service := fs.String("service", "", "서비스 이름. 비우면 같은 측정값의 기존 등록(있으면 누구 것이든)에서 가져온다")
	version := fs.String("version", "", "버전. 비우면 위와 같이 기존 등록에서 가져온다")
	expires := fs.Duration("expires", 90*24*time.Hour, "지금부터 얼마 뒤에 만료되나")
	trusted := fs.String("trusted", "", "신뢰할 서명자 주소, 콤마로 구분. 비우면 컨트랙트의 AUDITOR · VENDOR 를 그대로 쓴다")
	out := fs.String("out", "", "T1·T2 중간 파일을 둘 디렉터리. 비우면 임시 디렉터리를 쓰고 끝나면 지운다")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 옵션으로 안 준 값은 register 처럼 차례로 묻는다 — plain 에서 전부 플래그로 줄 필요는 없다.
	ask(rpc, "RPC 주소", "https://ethereum-rpc.publicnode.com")
	ask(contract, "Registry 컨트랙트 주소 (0x…)", "")
	ask(keyFile, "내 개인키 파일", "")
	if *tee == "" && *measurements == "" {
		ask(tee, "TEE 주소 host:port (비우면 측정값 파일을 쓴다)", "")
	}
	if *tee == "" && *measurements == "" {
		ask(measurements, "측정값 파일", "out/measurements.json")
	}
	if *rpc == "" || !common.IsHexAddress(*contract) || *keyFile == "" {
		return errors.New("--rpc, --contract, --key-file 는 필수")
	}
	if *tee == "" && *measurements == "" {
		return errors.New("--tee 또는 --measurements 가 필요하다")
	}

	measPath := *measurements
	if *tee != "" {
		dir := *out
		if dir == "" {
			tmp, err := os.MkdirTemp("", "tb-vendor-run-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			dir = tmp
		} else if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		docPath := filepath.Join(dir, "doc.cbor")
		measPath = filepath.Join(dir, "measurements.json")

		fmt.Println("== 1/3 T1 RA 요청")
		if err := raRequest([]string{"--tee", *tee, "--out", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
			return err
		}
		fmt.Println("\n== 2/3 T2 RA 파싱")
		parseArgs := []string{"--doc", docPath, "--out", measPath}
		if *nonce != "" {
			parseArgs = append(parseArgs, "--nonce", *nonce)
		}
		if err := raParse(parseArgs); err != nil {
			return err
		}
	} else {
		fmt.Println("== 1/3 (건너뜀 — --measurements 를 그대로 씀)")
		fmt.Println("== 2/3 (건너뜀)")
	}

	values, err := readMeasurements(measPath)
	if err != nil {
		return err
	}
	reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(*contract)}

	if *service == "" || *version == "" {
		fmt.Println("\n== service/version 이 비어 있음 — 같은 측정값의 기존 등록에서 가져온다")
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
			fmt.Printf("   %s %s (key %s 에서 가져옴)\n", existing.Meta.Service, existing.Meta.Version, k)
			resolved = true
			break
		}
		icancel()
		if !resolved {
			fmt.Println("   같은 측정값의 기존 등록이 없다 — 직접 입력하라")
		}
		ask(service, "서비스 이름", "")
		ask(version, "버전", "")
		if *service == "" || *version == "" {
			return errors.New("--service, --version 은 필수다")
		}
	}

	fmt.Println("\n== C1 등록 후 M2 자기확인")
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
