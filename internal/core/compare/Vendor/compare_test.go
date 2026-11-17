// Compare is a pure function and is tested directly. Check is tested against a registration actually made on the
// simulated chain and read back with chain/lookup, exactly as it is used in practice (compare a lookup result
// after registering).
package vendor

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"

	l "attestamp/internal/chain/lookup/Vendor"
	lether "attestamp/internal/chain/lookup/Vendor/ether"
	reg "attestamp/internal/chain/register/Vendor"
	regether "attestamp/internal/chain/register/Vendor/ether"
	p "attestamp/internal/tee/parse/Vendor"
)

func TestCompare_ParsedVsBuild(t *testing.T) {
	parsed := p.Measurements{{Name: "PCR0", Value: []byte{1, 2, 3}}}
	build := p.Measurements{{Name: "PCR0", Value: []byte{1, 2, 3}}}
	v := (Impl{}).Compare(parsed, build)
	if !v.Pass {
		t.Fatalf("same values reported as a mismatch: %+v", v)
	}

	tampered := p.Measurements{{Name: "PCR0", Value: []byte{9, 9, 9}}}
	v = (Impl{}).Compare(parsed, tampered)
	if v.Pass {
		t.Fatal("different values reported as a match")
	}
}

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

func TestCheck_AgainstRealChainRegistration(t *testing.T) {
	deployerKey, deployer := newKey(t)
	vendorKey, vendorAddr := newKey(t)
	client := chain(t, deployer, vendorAddr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	registryAddr, _, err := regether.Deploy(ctx, "", client, deployerKey, vendorAddr)
	if err != nil {
		t.Fatal(err)
	}
	values := p.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{0x11}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{0x22}, 48)},
	}
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	registrar := regether.Registrar{Backend: client, RegistryAddr: registryAddr, SenderKey: vendorKey}
	meta := reg.Meta{Meta: l.Meta{Service: "svc", Version: "1.0.0", Platform: lether.DefaultPlatform, Arch: "amd64"}, Expires: expires}
	signed, err := registrar.Sign(values, meta, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	reader := lether.Reader{RegistryAddr: registryAddr, Backend: client}
	regInfo, err := reader.Lookup(ctx, receipt.RecordKey)
	if err != nil {
		t.Fatal(err)
	}
	trustedApprovers, err := reader.TrustedApprovers(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Check with the value just registered; it must pass.
	if v := (Impl{}).Check(values, regInfo, time.Now().Unix(), trustedApprovers); !v.Pass {
		t.Fatalf("exactly the registered value failed: %+v", v)
	}

	// Check with a different value; the hash differs, so it must fail.
	other := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x99}, 48)}}
	if v := (Impl{}).Check(other, regInfo, time.Now().Unix(), trustedApprovers); v.Pass {
		t.Fatal("a different measurement passed")
	}

	// Check with a time past expiry; it must fail.
	future := expires.Add(time.Hour).Unix()
	if v := (Impl{}).Check(values, regInfo, future, trustedApprovers); v.Pass {
		t.Fatal("a time past expiry passed")
	}

	// A approver outside the trusted list; it must fail.
	if v := (Impl{}).Check(values, regInfo, time.Now().Unix(), []string{"0x0000000000000000000000000000000000000001"}); v.Pass {
		t.Fatal("a approver outside the trusted list passed")
	}
}
