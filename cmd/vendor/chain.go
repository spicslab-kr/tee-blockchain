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

	l "tb/internal/chain/lookup/Vendor"
	lether "tb/internal/chain/lookup/Vendor/ether"
	reg "tb/internal/chain/register/Vendor"
	"tb/internal/chain/register/Vendor/ether"
	p "tb/internal/tee/parse/Vendor"
)

func registryDeploy(args []string) error {
	fs := flag.NewFlagSet("registry-deploy", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	keyFile := fs.String("key-file", "", "private key file (hex) that sends the deploy transaction")
	approver := fs.String("approver", "", "address of the signing key this contract accepts (usually my signing key)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 옵션으로 안 준 값은 차례로 묻는다
	askMainnetRPC(rpc)
	ask(keyFile, "private key file that sends the deploy transaction", "")
	ask(approver, "address of the signing key this contract accepts (0x…)", "")
	if *rpc == "" || *keyFile == "" || !common.IsHexAddress(*approver) {
		return errors.New("--rpc, --key-file and --approver are required")
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

// register — C1. 등록에 쓴 key(조회 좌표)를 돌려준다 — run 이 이어서 바로 self-check compare 하는 데 쓴다.
func register(args []string) (string, error) {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	contract := fs.String("contract", "", "Registry contract address")
	keyFile := fs.String("key-file", "", "private key file (hex) that signs and sends the transaction")
	measurements := fs.String("measurements", "", "measurements: a nitro measurements.json file, or NAME=hex,NAME=hex")
	platform := fs.String("platform", "aws-nitro", "measurement format")
	service := fs.String("service", "", "service name")
	version := fs.String("version", "", "version")
	expires := fs.Duration("expires", 90*24*time.Hour, "how long from now until the registration expires")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	// 옵션으로 안 준 값은 차례로 묻는다
	askMainnetRPC(rpc)
	ask(contract, "Registry contract address (0x…)", "")
	ask(keyFile, "my private key file", "")
	ask(measurements, "measurements (path to measurements.json, or NAME=hex,…)", "out/measurements.json")
	ask(service, "service name", "")
	ask(version, "version", "")
	if *rpc == "" || !common.IsHexAddress(*contract) || *keyFile == "" || *measurements == "" || *service == "" || *version == "" {
		return "", errors.New("--rpc, --contract, --key-file, --measurements, --service and --version are required")
	}
	key, err := readKey(*keyFile)
	if err != nil {
		return "", err
	}
	values, err := readMeasurements(*measurements)
	if err != nil {
		return "", err
	}
	meta := reg.Meta{Meta: l.Meta{Service: *service, Version: *version, Platform: *platform}, Expires: time.Now().Add(*expires).Truncate(time.Second)}
	r := ether.Registrar{RPC: *rpc, Contract: common.HexToAddress(*contract), SenderKey: key}
	signed, err := r.Sign(values, meta, key)
	if err != nil {
		return "", err
	}
	fmt.Printf("signer %s\n", signed.Signer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	receipt, err := r.Register(ctx, signed)
	if err != nil {
		return "", err
	}
	fmt.Printf("registered\n  key     %s\n  tx      %s\n  block   %d\n  expires %s\n", receipt.Key, receipt.TxHash, receipt.Block, meta.Expires.UTC().Format(time.RFC3339))
	return receipt.Key, nil
}

// lookup — C2. --key 가 있으면 바로 그 key 를 읽는다. 없으면 --measurements 로 key 를 먼저 찾은 뒤 전부 읽는다.
// 찾은 key 들을 돌려준다 — run 이 이어서 각각 compare 하는 데 쓴다.
func lookup(args []string) ([]string, error) {
	fs := flag.NewFlagSet("lookup", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	contract := fs.String("contract", "", "Registry contract address")
	key := fs.String("key", "", "lookup key (0x…, the key from register). with it, --measurements is not needed")
	measurements := fs.String("measurements", "", "measurements: a measurements.json file, or NAME=hex,NAME=hex")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	ask(rpc, "RPC URL", "https://ethereum-rpc.publicnode.com")
	ask(contract, "Registry contract address (0x…)", "")
	if *rpc == "" || !common.IsHexAddress(*contract) {
		return nil, errors.New("--rpc and --contract are required")
	}
	reader := lether.Reader{RPC: *rpc, Contract: common.HexToAddress(*contract)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	keys := []string{*key}
	if *key == "" {
		if *measurements == "" {
			return nil, errors.New("--key or --measurements is required")
		}
		values, err := readMeasurements(*measurements)
		if err != nil {
			return nil, err
		}
		keys, err = reader.FindAddress(ctx, values)
		if err != nil {
			return nil, err
		}
		if len(keys) == 0 {
			fmt.Println("nothing registered")
			return nil, nil
		}
		fmt.Printf("found %d key(s)\n", len(keys))
	}
	for _, k := range keys {
		reg, err := reader.Lookup(ctx, k)
		if err != nil {
			return nil, err
		}
		fmt.Printf("key       %s\n", k)
		fmt.Printf("  signer  %s\n", reg.Signer)
		fmt.Printf("  service %s %s (%s)\n", reg.Meta.Service, reg.Meta.Version, reg.Meta.Platform)
		fmt.Printf("  expires %s\n", time.Unix(reg.Expires, 0).UTC().Format(time.RFC3339))
		if h := reg.Values.Get("measurementHash"); h != nil {
			fmt.Printf("  measurement hash  %s\n", hex.EncodeToString(h))
		}
	}
	return keys, nil
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

const mainnetRPC = "https://ethereum-rpc.publicnode.com"

// askMainnetRPC — 실제 자금이 오가는 트랜잭션(등록 · 배포)이라, mainnet 을 쓸 땐 그냥 엔터로 넘어가지
// 못하게 한다. 값이 있으면(플래그로 줬으면) 그대로 쓴다. 비어 있으면 y 를 쳐야 mainnet 을 쓰고, 다른
// 문자열을 치면 그걸 RPC 주소로 그대로 쓴다(Sepolia 같은 테스트넷도 이렇게 명시적으로 준다).
func askMainnetRPC(dst *string) {
	if *dst != "" {
		return
	}
	fmt.Fprintf(os.Stderr, "RPC URL — this sends a real transaction to mainnet (%s). y to use it, or type another RPC URL: ", mainnetRPC)
	line, _ := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	if strings.EqualFold(line, "y") {
		*dst = mainnetRPC
	} else {
		*dst = line
	}
}
