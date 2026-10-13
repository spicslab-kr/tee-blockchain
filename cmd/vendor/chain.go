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
	"tb/internal/cli"
	tr "tb/internal/core/transfer/Vendor"
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
	// ask for any value not given as an option, in turn
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

// register returns the key (lookup coordinate) used for the registration, so run can immediately self-check it with compare.
func register(args []string) (string, error) {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	contract := fs.String("contract", "", "Registry contract address")
	keyFile := fs.String("key-file", "", "private key file (hex) that signs and sends the transaction")
	measurements := fs.String("measurements", "", "measurements: a nitro measurements.json file, or NAME=hex,NAME=hex")
	platform := fs.String("platform", "aws-nitro", "measurement format")
	config := fs.String("config", "", "my build-config.json (the one send pushes): service, version and build_env are taken from it, so --service/--version/--arch are optional and only override it, with a warning")
	service := fs.String("service", "", "service name")
	version := fs.String("version", "", "version")
	arch := fs.String("arch", "", "CPU architecture the image was built for and measured on (amd64|arm64). measurements differ per architecture, so one service/version is registered once per architecture. default: build-config.json's with --config, else this machine's")
	expires := fs.Duration("expires", 90*24*time.Hour, "how long from now until the registration expires")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	cfg, err := loadBuildConfig(*config)
	if err != nil {
		return "", err
	}
	// ask for any value not given as an option, in turn; service and version are not asked when --config provides them
	askMainnetRPC(rpc)
	ask(contract, "Registry contract address (0x…)", "")
	ask(keyFile, "my private key file", "")
	ask(measurements, "measurements (path to measurements.json, or NAME=hex,…)", "out/measurements.json")
	if cfg == nil {
		ask(service, "service name", "")
		ask(version, "version", "")
	}
	if *rpc == "" || !common.IsHexAddress(*contract) || *keyFile == "" || *measurements == "" || (cfg == nil && (*service == "" || *version == "")) {
		return "", errors.New("--rpc, --contract, --key-file, --measurements, and --service and --version (or --config <build-config.json>, which carries them) are required")
	}
	if err := resolveRegistration(cfg, service, version, arch); err != nil {
		return "", err
	}
	if err := checkArch(*arch); err != nil {
		return "", err
	}
	key, err := readKey(*keyFile)
	if err != nil {
		return "", err
	}
	values, err := readMeasurements(*measurements)
	if err != nil {
		return "", err
	}
	meta := reg.Meta{Meta: l.Meta{Service: *service, Version: *version, Platform: *platform, Arch: *arch}, Expires: time.Now().Add(*expires).Truncate(time.Second)}
	r := ether.Registrar{RPC: *rpc, Contract: common.HexToAddress(*contract), SenderKey: key}
	signed, err := r.Sign(values, meta, key)
	if err != nil {
		return "", err
	}
	// the build environment this registration is for: --arch says which machine the image was built for and measured
	// on; the Go toolchain is this binary's — it is pinned project-wide (build.CheckToolchain at build start), so it is
	// the one that compiled tb-ra-responder into the image. with --config it is the config's build_env instead: that
	// describes the build that was measured, whichever machine runs register. of it, only the arch goes into the chain
	// Entry (meta.Arch above) — display only, so a reader can tell the two architectures' registrations apart; the key
	// still splits by the measurement hash alone. the rest is said here and in measurement-record.json.
	env := tr.CurrentBuildEnv()
	if cfg != nil {
		env = cfg.BuildEnv
	}
	env.Arch = *arch
	env.OS = "linux" // the enclave image is Linux whichever machine runs register
	goVer := env.Go
	if env.Trimpath {
		goVer += " -trimpath"
	}
	if !quiet {
		fmt.Printf("registering %s %s (%s, %s)\nsigner %s\n", *service, *version, *arch, goVer, signed.Signer)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	receipt, err := r.Register(ctx, signed)
	if err != nil {
		return "", err
	}
	lastRegistered.Key, lastRegistered.Tx, lastRegistered.Block = receipt.Key, receipt.TxHash, receipt.Block
	lastRegistered.Expires, lastRegistered.Signer, lastRegistered.BuildEnv = meta.Expires.UTC().Format(time.RFC3339), signed.Signer, env
	if !quiet {
		fmt.Printf("registered %s %s (%s, %s)\n  key     %s\n  tx      %s\n  block   %d\n  expires %s\n  build env  %s\n", *service, *version, *arch, goVer, receipt.Key, receipt.TxHash, receipt.Block, lastRegistered.Expires, env)
		fmt.Printf("  this registration is for %s only — put \"build_env\": %s in measurement-record.json, and register the other architecture's build separately\n", *arch, buildEnvJSON(env))
	}
	return receipt.Key, nil
}

// loadBuildConfig — --config: my build-config.json. Empty path → nil (flags only).
// DecodeBuildConfig validates it, so a config that lacks service, version or build_env is refused here.
func loadBuildConfig(path string) (*tr.BuildConfig, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := tr.DecodeBuildConfig(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// resolveRegistration — fills service/version/arch in place from the config when the flags are empty; a flag that is
// given and differs from the config wins, with one warning line. Without a config, --arch defaults to this machine's.
func resolveRegistration(cfg *tr.BuildConfig, service, version, arch *string) error {
	if cfg == nil && *arch == "" {
		*arch = defaultArch()
	}
	svc, ver, a, warnings, err := tr.ResolveRegistration(cfg, *service, *version, *arch)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		cli.Errorf("warning: %s", w)
	}
	*service, *version, *arch = svc, ver, a
	return nil
}

// buildEnvJSON — the record's build_env object on one line, as it should be pasted into measurement-record.json.
func buildEnvJSON(env tr.BuildEnv) string {
	b, err := json.Marshal(env)
	if err != nil {
		return env.String()
	}
	return string(b)
}

// lookup: with --key, reads that key directly. Otherwise it first finds the keys via --measurements, then reads them all.
// It returns the keys it found, so run can compare each of them.
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
		fmt.Printf("  service %s %s (%s, %s)\n", reg.Meta.Service, reg.Meta.Version, reg.Meta.Platform, reg.Meta.Arch)
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

// readMeasurements accepts nitro-cli's measurements.json ({"Measurements":{"PCR0":...}}), a flat {"PCR0":...}, or NAME=hex,...
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

// ask prompts if the value is empty. Plain Enter takes the default.
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

// askMainnetRPC: registration and deployment are transactions that move real funds, so mainnet is never reached by just pressing
// Enter. A value given as a flag is used as is. If empty, typing y selects mainnet and any other
// string is used as the RPC address (a testnet such as Sepolia is likewise given explicitly).
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
