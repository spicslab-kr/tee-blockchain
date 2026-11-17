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

	l "attestamp/internal/chain/lookup/auditor"
	reg "attestamp/internal/chain/register/auditor"
	regether "attestamp/internal/chain/register/auditor/ether"
	p "attestamp/internal/tee/parse/auditor"
)

func newKey(t *testing.T) ([]byte, common.Address) {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return crypto.FromECDSA(k), crypto.PubkeyToAddress(k.PublicKey)
}

// chain is a simulated chain with no block producer in the background. Every transaction the returned client sends is
// sealed into a block at once, on the goroutine that sent it, and a test that wants more blocks calls Commit itself.
// It used to commit every 50 ms from a goroutine of its own. go-ethereum's SimulatedBeacon.Commit takes no lock, and
// TxPool.Sync, which every Commit calls, keeps a single waiter: when that goroutine and a test's own Commit ran at
// once, one of them waited forever. On 2026-11-17 that held TestReader_Lookup_ScansMultipleLogChunks at its own
// Commit until go test's 10-minute limit, in a full run with the other packages in parallel.
func chain(t *testing.T, funded ...common.Address) sealing {
	t.Helper()
	alloc := types.GenesisAlloc{}
	for _, a := range funded {
		alloc[a] = types.Account{Balance: new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))}
	}
	b := simulated.NewBackend(alloc)
	t.Cleanup(func() { _ = b.Close() })
	return sealing{Client: b.Client(), b: b}
}

// sealing is the simulated chain's client, except that SendTransaction also seals the transaction into a block, so
// bind.WaitMined and bind.WaitDeployed return without anything committing in the background.
type sealing struct {
	simulated.Client
	b *simulated.Backend
}

func (s sealing) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := s.Client.SendTransaction(ctx, tx); err != nil {
		return err
	}
	s.b.Commit()
	return nil
}

// Commit seals one more block.
func (s sealing) Commit() { s.b.Commit() }

func TestReader_FindRecordKeysAndLookup(t *testing.T) {
	deployerKey, deployer := newKey(t)
	auditorKey, auditorAddr := newKey(t)
	client := chain(t, deployer, auditorAddr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := regether.Deploy(ctx, "", client, deployerKey, auditorAddr)
	if err != nil {
		t.Fatal(err)
	}

	values := p.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{0x11}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{0x22}, 48)},
		{Name: "PCR2", Value: bytes.Repeat([]byte{0x33}, 48)},
	}
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	registrar := regether.Registrar{Backend: client, RegistryAddr: contract, SenderKey: auditorKey}
	meta := reg.Meta{Meta: l.Meta{Service: "unit-test", Version: "0.1.0", Platform: DefaultPlatform, Arch: "arm64"}, Expires: expires}
	signed, err := registrar.Sign(values, meta, auditorKey)
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
	if got.Approver != auditorAddr.Hex() || got.Meta.Service != "unit-test" || got.Meta.Platform != DefaultPlatform || got.Expires != expires.Unix() {
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
	auditorKey, auditorAddr := newKey(t)
	client := chain(t, deployer, auditorAddr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := regether.Deploy(ctx, "", client, deployerKey, auditorAddr)
	if err != nil {
		t.Fatal(err)
	}
	values := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x44}, 48)}}
	registrar := regether.Registrar{Backend: client, RegistryAddr: contract, SenderKey: auditorKey}
	meta := reg.Meta{Meta: l.Meta{Service: "chunk-test", Version: "0.1.0", Platform: DefaultPlatform, Arch: "amd64"}, Expires: time.Now().Add(time.Hour)}
	signed, err := registrar.Sign(values, meta, auditorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	// Pile up several more blocks after registration: on a real chain time passes and the latest block is far ahead of the registration.
	for i := 0; i < 8; i++ {
		client.Commit()
	}
	// The blocks come only from the commits above, so the scan has exactly these eight to step back through.
	if latest, err := client.BlockNumber(ctx); err != nil || latest != receipt.Block+8 {
		t.Fatalf("latest block = %d (%v), want the registration's block %d + 8", latest, err, receipt.Block)
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

// The record key includes the platform, so a registration made under intel-tdx is invisible to a Reader looking under
// the default (aws-nitro) and found by one whose Platform names it.
func TestReader_FindRecordKeys_PerPlatform(t *testing.T) {
	deployerKey, deployer := newKey(t)
	approverKey, approverAddr := newKey(t)
	client := chain(t, deployer, approverAddr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := regether.Deploy(ctx, "", client, deployerKey, approverAddr)
	if err != nil {
		t.Fatal(err)
	}
	values := p.Measurements{
		{Name: "MRTD", Value: bytes.Repeat([]byte{0x44}, 48)},
		{Name: "RTMR0", Value: bytes.Repeat([]byte{0x55}, 48)},
		{Name: "RTMR1", Value: bytes.Repeat([]byte{0x66}, 48)},
		{Name: "RTMR2", Value: bytes.Repeat([]byte{0x77}, 48)},
	}
	registrar := regether.Registrar{Backend: client, RegistryAddr: contract, SenderKey: approverKey}
	meta := reg.Meta{Meta: l.Meta{Service: "tdx-test", Version: "0.1.0", Platform: "intel-tdx", Arch: "amd64"}, Expires: time.Now().Add(time.Hour)}
	signed, err := registrar.Sign(values, meta, approverKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	if got, err := (Reader{RegistryAddr: contract, Backend: client}).FindRecordKeys(ctx, values); err != nil || len(got) != 0 {
		t.Fatalf("under the default platform the intel-tdx registration must not be found: %v, %v", got, err)
	}
	got, err := (Reader{RegistryAddr: contract, Backend: client, Platform: "intel-tdx"}).FindRecordKeys(ctx, values)
	if err != nil || len(got) != 1 || got[0] != receipt.RecordKey {
		t.Fatalf("FindRecordKeys under intel-tdx = %v, %v, want [%s]", got, err, receipt.RecordKey)
	}
}
