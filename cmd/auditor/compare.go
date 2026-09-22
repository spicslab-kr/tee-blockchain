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
	cmp "tb/internal/core/compare/auditor"
)

// compareCmd — M2. --b 가 있으면 측정값 두 묶음을 그대로 맞댄다(Compare). --key 가 있으면 체인에서
// 등록 정보를 읽어 만료 · 신뢰 서명자까지 같이 본다(Check).
func compareCmd(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	a := fs.String("a", "", "측정값 A: measurements.json 파일, 또는 NAME=hex,NAME=hex")
	b := fs.String("b", "", "측정값 B — --key 를 안 쓰면 필수")
	rpc := fs.String("rpc", "", "체인 모드: 이더리움 RPC 주소")
	contract := fs.String("contract", "", "체인 모드: Registry 컨트랙트 주소")
	key := fs.String("key", "", "체인 모드: 조회 key (register 결과의 key)")
	trusted := fs.String("trusted", "", "체인 모드: 신뢰할 서명자 주소, 콤마로 구분. 비우면 컨트랙트의 AUDITOR · VENDOR 를 그대로 쓴다")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *a == "" {
		return errors.New("--a 는 필수")
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
			return errors.New("체인 모드엔 --rpc, --contract, --key 가 다 있어야 한다")
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
		return errors.New("--b 또는 (--contract 와 --key) 가 필요하다")
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
		return errors.New("--b(평문 비교)와 --key(체인 조회)를 같이 주면 안 된다 — 하나만 골라라")
	case b != "" && chainFlags:
		return errors.New("--b 를 줬는데 --rpc/--contract/--trusted 도 같이 줬다 — --key 없인 체인 모드가 아니니 지운다")
	case key == "" && (rpc != "" || contract != ""):
		return errors.New("--rpc 또는 --contract 를 줬으면 --key 도 있어야 체인 모드다")
	case key == "" && trusted != "":
		return errors.New("--trusted 는 --key 없이는 쓰이지 않는다 (체인 모드 전용)")
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
	fmt.Println("FAIL")
	for _, r := range v.Reason {
		fmt.Println("  -", r)
	}
	os.Exit(1)
	return nil
}
