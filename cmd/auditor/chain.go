package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	l "tb/internal/chain/lookup/auditor"
	lether "tb/internal/chain/lookup/auditor/ether"
	reg "tb/internal/chain/register/auditor"
	"tb/internal/chain/register/auditor/ether"
	p "tb/internal/tee/parse/auditor"
)

func registryDeploy(args []string) error {
	fs := flag.NewFlagSet("registry-deploy", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "이더리움 RPC 주소")
	keyFile := fs.String("key-file", "", "배포 트랜잭션을 보낼 개인키 파일 (hex)")
	approver := fs.String("approver", "", "이 컨트랙트가 받을 서명 키의 주소 (보통 내 서명 키)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 옵션으로 안 준 값은 차례로 묻는다
	ask(rpc, "RPC 주소", "https://ethereum-sepolia-rpc.publicnode.com")
	ask(keyFile, "배포 트랜잭션을 보낼 개인키 파일", "")
	ask(approver, "이 컨트랙트가 받을 서명 키의 주소 (0x…)", "")
	if *rpc == "" || *keyFile == "" || !common.IsHexAddress(*approver) {
		return errors.New("--rpc, --key-file, --approver 는 필수")
	}
	key, err := readKey(*keyFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	addr, tx, err := ether.Deploy(ctx, *rpc, nil, key, common.HexToAddress(*approver))
	if err != nil {
		return err
	}
	fmt.Printf("Registry deployed at %s (tx %s)\napprover %s\n", addr.Hex(), tx, *approver)
	return nil
}

func register(args []string) error {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "이더리움 RPC 주소")
	contract := fs.String("contract", "", "Registry 컨트랙트 주소")
	keyFile := fs.String("key-file", "", "서명하고 트랜잭션을 보낼 개인키 파일 (hex)")
	measurements := fs.String("measurements", "", "측정값: nitro measurements.json 파일, 또는 NAME=hex,NAME=hex")
	platform := fs.String("platform", "aws-nitro", "측정값 형식")
	service := fs.String("service", "", "서비스 이름")
	version := fs.String("version", "", "버전")
	expires := fs.Duration("expires", 90*24*time.Hour, "지금부터 얼마 뒤에 만료되나")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 옵션으로 안 준 값은 차례로 묻는다
	ask(rpc, "RPC 주소", "https://ethereum-sepolia-rpc.publicnode.com")
	ask(contract, "Registry 컨트랙트 주소 (0x…)", "")
	ask(keyFile, "내 개인키 파일", "")
	ask(measurements, "측정값 (measurements.json 경로 또는 NAME=hex,…)", "out/measurements.json")
	ask(service, "서비스 이름", "")
	ask(version, "버전", "")
	if *rpc == "" || !common.IsHexAddress(*contract) || *keyFile == "" || *measurements == "" || *service == "" || *version == "" {
		return errors.New("--rpc, --contract, --key-file, --measurements, --service, --version 는 필수")
	}
	key, err := readKey(*keyFile)
	if err != nil {
		return err
	}
	values, err := readMeasurements(*measurements)
	if err != nil {
		return err
	}
	meta := reg.Meta{Meta: l.Meta{Service: *service, Version: *version, Platform: *platform}, Expires: time.Now().Add(*expires).Truncate(time.Second)}
	r := ether.Registrar{RPC: *rpc, Contract: common.HexToAddress(*contract), SenderKey: key}
	signed, err := r.Sign(values, meta, key)
	if err != nil {
		return err
	}
	fmt.Printf("signer %s\n", signed.Signer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	receipt, err := r.Register(ctx, signed)
	if err != nil {
		return err
	}
	fmt.Printf("registered\n  key     %s\n  tx      %s\n  block   %d\n  expires %s\n", receipt.Key, receipt.TxHash, receipt.Block, meta.Expires.UTC().Format(time.RFC3339))
	return nil
}

// lookup — C2. --key 가 있으면 바로 그 key 를 읽는다. 없으면 --measurements 로 key 를 먼저 찾은 뒤 전부 읽는다.
func lookup(args []string) error {
	fs := flag.NewFlagSet("lookup", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "이더리움 RPC 주소")
	contract := fs.String("contract", "", "Registry 컨트랙트 주소")
	key := fs.String("key", "", "조회 key (0x…, register 결과의 key). 있으면 --measurements 는 안 봐도 된다")
	measurements := fs.String("measurements", "", "측정값: measurements.json 파일, 또는 NAME=hex,NAME=hex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ask(rpc, "RPC 주소", "https://ethereum-sepolia-rpc.publicnode.com")
	ask(contract, "Registry 컨트랙트 주소 (0x…)", "")
	if *rpc == "" || !common.IsHexAddress(*contract) {
		return errors.New("--rpc, --contract 는 필수")
	}
	reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(*contract)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	keys := []string{*key}
	if *key == "" {
		if *measurements == "" {
			return errors.New("--key 또는 --measurements 가 필요하다")
		}
		values, err := readMeasurements(*measurements)
		if err != nil {
			return err
		}
		keys, err = reader.FindAddress(ctx, values)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			fmt.Println("등록된 것이 없다")
			return nil
		}
		fmt.Printf("found %d key(s)\n", len(keys))
	}
	for _, k := range keys {
		reg, err := reader.Lookup(ctx, k)
		if err != nil {
			return err
		}
		fmt.Printf("key       %s\n", k)
		fmt.Printf("  signer  %s\n", reg.Signer)
		fmt.Printf("  service %s %s (%s)\n", reg.Meta.Service, reg.Meta.Version, reg.Meta.Platform)
		fmt.Printf("  expires %s\n", time.Unix(reg.Expires, 0).UTC().Format(time.RFC3339))
		if h := reg.Values.Get("measurementHash"); h != nil {
			fmt.Printf("  measurement hash  %s\n", hex.EncodeToString(h))
		}
	}
	return nil
}

func readKey(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := strings.TrimPrefix(strings.TrimSpace(string(b)), "0x")
	key, err := hex.DecodeString(s)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%s: want a 32-byte hex private key", path)
	}
	return key, nil
}

// readMeasurements — nitro-cli 의 measurements.json({"Measurements":{"PCR0":…}}), 납작한 {"PCR0":…}, 또는 NAME=hex,… 를 받는다.
func readMeasurements(src string) (p.Measurements, error) {
	var raw map[string]string
	if b, err := os.ReadFile(src); err == nil {
		var wrapped struct {
			Measurements map[string]string
		}
		if json.Unmarshal(b, &wrapped) == nil && len(wrapped.Measurements) > 0 {
			raw = wrapped.Measurements
		} else if err := json.Unmarshal(b, &raw); err != nil {
			return nil, fmt.Errorf("%s: %w", src, err)
		}
	} else {
		raw = map[string]string{}
		for _, kv := range strings.Split(src, ",") {
			name, val, ok := strings.Cut(kv, "=")
			if !ok {
				return nil, fmt.Errorf("--measurements: %q is neither a readable file nor NAME=hex", src)
			}
			raw[strings.TrimSpace(name)] = strings.TrimSpace(val)
		}
	}
	var out p.Measurements
	for name, val := range raw {
		if name == "HashAlgorithm" {
			continue
		}
		v, err := hex.DecodeString(strings.TrimPrefix(val, "0x"))
		if err != nil {
			return nil, fmt.Errorf("measurement %s: %w", name, err)
		}
		out = append(out, p.Measurement{Name: name, Value: v})
	}
	return out, nil
}

var stdin = bufio.NewReader(os.Stdin)

// ask — 값이 비어 있으면 묻는다. 그냥 엔터면 기본값.
func ask(dst *string, label, def string) {
	if *dst != "" {
		return
	}
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	line, _ := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		line = def
	}
	*dst = line
}
