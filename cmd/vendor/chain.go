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

	l "attestamp/internal/chain/lookup/Vendor"
	lether "attestamp/internal/chain/lookup/Vendor/ether"
	reg "attestamp/internal/chain/register/Vendor"
	"attestamp/internal/chain/register/Vendor/ether"
	"attestamp/internal/cli"
	tr "attestamp/internal/core/transfer/Vendor"
	p "attestamp/internal/tee/parse/Vendor"
)

func registryDeploy(args []string) error {
	fs := flag.NewFlagSet("registry-deploy", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	deployerKeyFile := fs.String("deployer-key-file", "", "private key file (hex) of the deployer key, which only pays gas for the deploy transaction")
	approver := fs.String("approver", "", "address of the signing key this registry accepts (usually my signing key's address)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// ask for any value not given as an option, in turn
	askMainnetRPC(rpc)
	ask(deployerKeyFile, "deployer key file (pays gas for the deploy transaction)", "")
	ask(approver, "address of the signing key this registry accepts (0x…)", "")
	if *rpc == "" || *deployerKeyFile == "" || !common.IsHexAddress(*approver) {
		return errors.New("--rpc, --deployer-key-file and --approver are required")
	}
	deployerKey, err := readKey(*deployerKeyFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	addr, tx, err := ether.Deploy(ctx, *rpc, nil, deployerKey, common.HexToAddress(*approver))
	if err != nil {
		return err
	}
	fmt.Printf("Registry deployed at %s (tx %s)\napprover %s\n", addr.Hex(), tx, *approver)
	return nil
}

// register returns the record key (lookup coordinate) of the registration, so the caller can read it back from the chain.
func register(args []string) (string, error) {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	registry := fs.String("registry", "", "Registry contract address")
	signingKeyFile := fs.String("signing-key-file", "", "signing key file (hex): its signature authorises the registration, and the same key sends the transaction")
	measurements := fs.String("measurements", "", "measurements: a nitro measurements.json file, or NAME=hex,NAME=hex")
	platform := fs.String("platform", "aws-nitro", "platform value recorded in the registration")
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
	ask(registry, "Registry address (0x…)", "")
	ask(signingKeyFile, "my signing key file", "")
	ask(measurements, "measurements (path to measurements.json, or NAME=hex,…)", "out/measurements.json")
	if cfg == nil {
		ask(service, "service name", "")
		ask(version, "version", "")
	}
	if *rpc == "" || !common.IsHexAddress(*registry) || *signingKeyFile == "" || *measurements == "" || (cfg == nil && (*service == "" || *version == "")) {
		return "", errors.New("--rpc, --registry, --signing-key-file, --measurements, and --service and --version (or --config <build-config.json>, which carries them) are required")
	}
	if err := resolveRegistration(cfg, service, version, arch); err != nil {
		return "", err
	}
	if err := checkArch(*arch); err != nil {
		return "", err
	}
	signingKey, err := readKey(*signingKeyFile)
	if err != nil {
		return "", err
	}
	values, err := readMeasurements(*measurements)
	if err != nil {
		return "", err
	}
	meta := reg.Meta{Meta: l.Meta{Service: *service, Version: *version, Platform: *platform, Arch: *arch}, Expires: time.Now().Add(*expires).Truncate(time.Second)}
	r := ether.Registrar{RPC: *rpc, RegistryAddr: common.HexToAddress(*registry), SenderKey: signingKey}
	signed, err := r.Sign(values, meta, signingKey)
	if err != nil {
		return "", err
	}
	// env is the build environment of this registration. With --config it is the config's build_env. Without it, the
	// Go toolchain is this binary's (pinned project-wide) and the arch is --arch. Only the arch goes on the chain
	// (meta.Arch), so readers can tell the architectures apart; the record key depends on the measurement hash alone.
	// The full env is printed here and belongs in measurement-record.json.
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
		fmt.Printf("registering %s %s (%s, %s)\napprover %s\n", *service, *version, *arch, goVer, signed.Approver)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	receipt, err := r.Register(ctx, signed)
	if err != nil {
		return "", err
	}
	lastRegistered.RecordKey, lastRegistered.Tx, lastRegistered.Block = receipt.RecordKey, receipt.TxHash, receipt.Block
	lastRegistered.Expires, lastRegistered.Approver, lastRegistered.BuildEnv = meta.Expires.UTC().Format(time.RFC3339), signed.Approver, env
	if !quiet {
		fmt.Printf("registered %s %s (%s, %s)\n  record key  %s\n  tx          %s\n  block       %d\n  expires     %s\n  build env   %s\n", *service, *version, *arch, goVer, receipt.RecordKey, receipt.TxHash, receipt.Block, lastRegistered.Expires, env)
		fmt.Printf("  this registration is for %s only. Put \"build_env\": %s in measurement-record.json, and register the other architecture's build separately\n", *arch, buildEnvJSON(env))
	}
	return receipt.RecordKey, nil
}

// loadBuildConfig: --config, my build-config.json. Empty path → nil (flags only).
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

// resolveRegistration fills service/version/arch in place from the config when the flags are empty; a flag that is
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

// buildEnvJSON returns the record's build_env object on one line, as it should be pasted into measurement-record.json.
func buildEnvJSON(env tr.BuildEnv) string {
	b, err := json.Marshal(env)
	if err != nil {
		return env.String()
	}
	return string(b)
}

// lookup reads one record key directly with --record-key. With --measurements it first finds the record keys for them, then reads them all.
// It returns the record keys it found.
func lookup(args []string) ([]string, error) {
	fs := flag.NewFlagSet("lookup", flag.ContinueOnError)
	rpc := fs.String("rpc", "", "Ethereum RPC URL")
	registry := fs.String("registry", "", "Registry contract address")
	recordKey := fs.String("record-key", "", "record key (0x…, the one register printed). with it, --measurements is not needed")
	measurements := fs.String("measurements", "", "measurements: a measurements.json file, or NAME=hex,NAME=hex")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	ask(rpc, "RPC URL", "https://ethereum-rpc.publicnode.com")
	ask(registry, "Registry address (0x…)", "")
	if *rpc == "" || !common.IsHexAddress(*registry) {
		return nil, errors.New("--rpc and --registry are required")
	}
	reader := lether.Reader{RPC: *rpc, RegistryAddr: common.HexToAddress(*registry)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	recordKeys := []string{*recordKey}
	if *recordKey == "" {
		if *measurements == "" {
			return nil, errors.New("--record-key or --measurements is required")
		}
		values, err := readMeasurements(*measurements)
		if err != nil {
			return nil, err
		}
		recordKeys, err = reader.FindRecordKeys(ctx, values)
		if err != nil {
			return nil, err
		}
		if len(recordKeys) == 0 {
			if !quiet {
				fmt.Println("nothing registered")
			}
			return nil, nil
		}
		if !quiet {
			fmt.Printf("found %d record key(s)\n", len(recordKeys))
		}
	}
	for _, k := range recordKeys {
		reg, err := reader.Lookup(ctx, k)
		if err != nil {
			return nil, err
		}
		if quiet {
			continue
		}
		fmt.Printf("record key  %s\n", k)
		fmt.Printf("  approver  %s\n", reg.Approver)
		fmt.Printf("  service   %s %s (%s, %s)\n", reg.Meta.Service, reg.Meta.Version, reg.Meta.Platform, reg.Meta.Arch)
		fmt.Printf("  expires   %s\n", time.Unix(reg.Expires, 0).UTC().Format(time.RFC3339))
		if h := reg.Values.Get("measurementHash"); h != nil {
			fmt.Printf("  measurement hash  %s\n", hex.EncodeToString(h))
		}
	}
	return recordKeys, nil
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

// askMainnetRPC asks for the RPC URL when --rpc is empty. Registration and deployment are transactions that move real funds, so
// mainnet needs an explicit y: any other input is used as the RPC address (a testnet such as Sepolia is given this way).
func askMainnetRPC(dst *string) {
	if *dst != "" {
		return
	}
	fmt.Fprintf(os.Stderr, "RPC URL: this sends a real transaction to mainnet (%s). y to use it, or type another RPC URL: ", mainnetRPC)
	line, _ := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	if strings.EqualFold(line, "y") {
		*dst = mainnetRPC
	} else {
		*dst = line
	}
}
