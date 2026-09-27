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

	lether "tb/internal/chain/lookup/user/ether"
	p "tb/internal/tee/parse/user"
)

// lookup — C2. User 는 등록을 안 하니 registry-deploy · register 는 없다. --key 가 있으면 바로 그 key 를
// 읽는다. 없으면 --measurements 로 key 를 먼저 찾은 뒤 전부 읽는다. 찾은 key 들을 돌려준다 — run 이
// 이어서 각각 compare 하는 데 쓴다.
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
			if !quiet {
				fmt.Println("nothing registered")
			}
			return nil, nil
		}
		if !quiet {
			fmt.Printf("found %d key(s)\n", len(keys))
		}
	}
	for _, k := range keys {
		reg, err := reader.Lookup(ctx, k)
		if err != nil {
			return nil, err
		}
		if quiet {
			continue
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
