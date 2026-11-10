// Registers on a simulated chain with register, then looks up with Reader. chain() and newKey() have the same shape as the
// ones in register's ether_test.go, but this is a different package so they cannot be reused and are written out again.
package ether

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"

	l "attestamp/internal/chain/lookup/Vendor"
	reg "attestamp/internal/chain/register/Vendor"
	regether "attestamp/internal/chain/register/Vendor/ether"
	p "attestamp/internal/tee/parse/Vendor"
)

func newKey(t *testing.T) ([]byte, common.Address) {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return crypto.FromECDSA(k), crypto.PubkeyToAddress(k.PublicKey)
}

func chain(t *testing.T, funded ...common.Address) *simulated.Backend {
	t.Helper()
	alloc := types.GenesisAlloc{}
	for _, a := range funded {
		alloc[a] = types.Account{Balance: new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))}
	}
	b := simulated.NewBackend(alloc)
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				b.Commit()
			}
		}
	}()
	t.Cleanup(func() { close(stop); _ = b.Close() })
	return b
}

func TestReader_FindRecordKeysAndLookup(t *testing.T) {
	deployerKey, deployer := newKey(t)
	vendorKey, vendorAddr := newKey(t)
	client := chain(t, deployer, vendorAddr).Client()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := regether.Deploy(ctx, "", client, deployerKey, vendorAddr)
	if err != nil {
		t.Fatal(err)
	}

	values := p.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{0x11}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{0x22}, 48)},
		{Name: "PCR2", Value: bytes.Repeat([]byte{0x33}, 48)},
	}
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	registrar := regether.Registrar{Backend: client, RegistryAddr: contract, SenderKey: vendorKey}
	meta := reg.Meta{Meta: l.Meta{Service: "unit-test", Version: "0.1.0", Platform: DefaultPlatform, Arch: "arm64"}, Expires: expires}
	signed, err := registrar.Sign(values, meta, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	reader := Reader{RegistryAddr: contract, Backend: client}

	keys, err := reader.FindRecordKeys(ctx, values)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != receipt.RecordKey {
		t.Fatalf("FindRecordKeys = %v, want [%s]", keys, receipt.RecordKey)
	}

	got, err := reader.Lookup(ctx, receipt.RecordKey)
	if err != nil {
		t.Fatal(err)
	}
	if got.Approver != vendorAddr.Hex() || got.Meta.Service != "unit-test" || got.Meta.Platform != DefaultPlatform || got.Expires != expires.Unix() {
		t.Fatalf("Lookup = %+v", got)
	}
	// chain clock used for the expiry decision: the time of the block that served the lookup. If 0, Check falls back to the local clock.
	if got.ChainTime <= 0 || got.ChainTime > time.Now().Unix()+60 {
		t.Fatalf("Lookup ChainTime = %d, want the block's unix time", got.ChainTime)
	}
	// the chain says which architecture the registration is for; it is read straight from the event.
	if got.Meta.Arch != "arm64" {
		t.Fatalf("Lookup arch = %q, want arm64", got.Meta.Arch)
	}
	h, _ := reg.MeasurementHash(values)
	if !bytes.Equal(got.Values.Get("measurementHash"), h[:]) {
		t.Error("measurement hash differs from the one computed at registration")
	}

	// unregistered value: empty slice.
	other := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x99}, 48)}}
	if got, err := reader.FindRecordKeys(ctx, other); err != nil || len(got) != 0 {
		t.Fatalf("FindRecordKeys for an unregistered measurement = %v, %v", got, err)
	}

	// unregistered record key: error.
	if _, err := reader.Lookup(ctx, common.HexToHash("0x3039").Hex()); err == nil {
		t.Fatal("an unregistered record key should be an error")
	}

	// a record key of the right length that is not hex: it must not silently become 0 and pass as "not registered"; it must be an error in itself.
	bad := "0x" + strings.Repeat("zz", 32)
	if _, err := reader.Lookup(ctx, bad); err == nil {
		t.Fatal("a non-hex record key should be an error (it must not silently turn into 0)")
	}
}

// TestReader_Lookup_ScansMultipleLogChunks imitates a real chain where blocks keep piling up after the registration.
// LogRangeChunk is made very small so the event cannot be found in one query from the latest block and the scan has to step
// back several times, exercising that path. With the default chunk of 4000, a simulated chain's few blocks are always found
// in one query.
func TestReader_Lookup_ScansMultipleLogChunks(t *testing.T) {
	deployerKey, deployer := newKey(t)
	vendorKey, vendorAddr := newKey(t)
	sim := chain(t, deployer, vendorAddr)
	client := sim.Client()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := regether.Deploy(ctx, "", client, deployerKey, vendorAddr)
	if err != nil {
		t.Fatal(err)
	}
	values := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x44}, 48)}}
	registrar := regether.Registrar{Backend: client, RegistryAddr: contract, SenderKey: vendorKey}
	meta := reg.Meta{Meta: l.Meta{Service: "chunk-test", Version: "0.1.0", Platform: DefaultPlatform, Arch: "amd64"}, Expires: time.Now().Add(time.Hour)}
	signed, err := registrar.Sign(values, meta, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	// Pile up several more blocks after registration: on a real chain time passes and the latest block is far ahead of the registration.
	for i := 0; i < 8; i++ {
		sim.Commit()
	}

	reader := Reader{RegistryAddr: contract, Backend: client, LogRangeChunk: 1}
	got, err := reader.Lookup(ctx, receipt.RecordKey)
	if err != nil {
		t.Fatalf("scan failed when stepping back one chunk at a time: %v", err)
	}
	if got.Meta.Service != "chunk-test" {
		t.Fatalf("Lookup = %+v", got)
	}
}
