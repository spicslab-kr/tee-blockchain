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
	cmp "tb/internal/core/compare/user"
)

// runCmd — 흐름1의 User 구간(14~18)을 명령 하나로: T1 요청 → T2 파싱 → C2 조회 → 찾은 key 마다 M2 대조.
// 하나라도 PASS 면 그 엔클레이브를 신뢰할 근거가 있다는 뜻이라 exit 0, 하나도 없거나 다 FAIL 이면 exit 1.
func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	tee := fs.String("tee", "", "TEE 주소 host:port")
	format := fs.String("format", "aws-nitro", "기대하는 문서 형식")
	timeout := fs.Duration("timeout", 30*time.Second, "RA 요청 타임아웃")
	nonce := fs.String("nonce", "", "고정된 nonce(테스트용 fixture 등). 비우면 방금 보낸 것을 쓴다")
	rpc := fs.String("rpc", "", "이더리움 RPC 주소")
	contract := fs.String("contract", "", "Registry 컨트랙트 주소")
	trusted := fs.String("trusted", "", "신뢰할 서명자 주소, 콤마로 구분. 비우면 컨트랙트의 AUDITOR · VENDOR 를 그대로 쓴다")
	out := fs.String("out", "", "중간 파일(문서 · 측정값)을 둘 디렉터리. 비우면 임시 디렉터리를 쓰고 끝나면 지운다")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 옵션으로 안 준 값은 lookup 처럼 차례로 묻는다.
	ask(tee, "TEE 주소 host:port", "")
	ask(rpc, "RPC 주소", "https://ethereum-rpc.publicnode.com")
	ask(contract, "Registry 컨트랙트 주소 (0x…)", "")
	if *tee == "" || *rpc == "" || !common.IsHexAddress(*contract) {
		return errors.New("--tee, --rpc, --contract 는 필수")
	}

	dir := *out
	if dir == "" {
		tmp, err := os.MkdirTemp("", "tb-user-run-*")
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

	fmt.Println("== 1/4 T1 RA 요청")
	if err := raRequest([]string{"--tee", *tee, "--out", docPath, "--format", *format, "--timeout", timeout.String()}); err != nil {
		return err
	}

	fmt.Println("\n== 2/4 T2 RA 파싱")
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

	fmt.Println("\n== 3/4 C2 체인 조회")
	keys, err := lookup([]string{"--rpc", *rpc, "--contract", *contract, "--measurements", measPath})
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return errors.New("체인에 이 측정값을 등록한 사람이 없다 — 신뢰할 근거가 없다")
	}

	fmt.Println("\n== 4/4 M2 대조 (조회값 vs 파싱값, 키마다)")
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
		fmt.Printf("  %s  FAIL\n", k)
		for _, r := range v.Reason {
			fmt.Println("    -", r)
		}
	}
	if !anyPass {
		fmt.Println("\n어느 등록도 이 attestation과 맞지 않는다")
		os.Exit(1)
	}
	fmt.Println("\n신뢰할 수 있다 — 최소 한 등록이 이 attestation과 일치한다")
	return nil
}
