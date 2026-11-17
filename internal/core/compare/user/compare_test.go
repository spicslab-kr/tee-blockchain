// Compare is a pure function and is tested directly. Check is tested against a registration actually made on the
// simulated chain and read back with chain/lookup. The User has no register/user package:
// the test registers through the auditor package and reads with the user's Reader (which borrows the auditor binding).
package user

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

	al "attestamp/internal/chain/lookup/auditor"
	l "attestamp/internal/chain/lookup/user"
	lether "attestamp/internal/chain/lookup/user/ether"
	aud "attestamp/internal/chain/register/auditor"
	audether "attestamp/internal/chain/register/auditor/ether"
	ap "attestamp/internal/tee/parse/auditor"
	p "attestamp/internal/tee/parse/user"
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

// The User's final check: compare freshly received RA values from the deployed service against the on-chain registration.
func TestCheck_AgainstRealChainRegistration(t *testing.T) {
	deployerKey, deployer := newKey(t)
	auditorKey, auditorAddr := newKey(t)
	client := chain(t, deployer, auditorAddr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	registryAddr, _, err := audether.Deploy(ctx, "", client, deployerKey, auditorAddr)
	if err != nil {
		t.Fatal(err)
	}
	aValues := ap.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{0x11}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{0x22}, 48)},
	}
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	registrar := audether.Registrar{Backend: client, RegistryAddr: registryAddr, SenderKey: auditorKey}
	meta := aud.Meta{Meta: al.Meta{Service: "svc", Version: "1.0.0", Platform: lether.DefaultPlatform, Arch: "amd64"}, Expires: expires}
	signed, err := registrar.Sign(aValues, meta, auditorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	// Pretend the User just parsed it in its own type: rebuild the same values as user.Measurements.
	values := make(p.Measurements, len(aValues))
	for i, m := range aValues {
		values[i] = p.Measurement{Name: m.Name, Value: m.Value}
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

	if regInfo.ChainTime <= 0 {
		t.Fatalf("Lookup did not fill in ChainTime: %+v", regInfo)
	}
	// The real call path: now is the lookup block's time, not the local clock.
	if v := (Impl{}).Check(values, regInfo, Clock(regInfo), trustedApprovers); !v.Pass {
		t.Fatalf("exactly the registered value failed: %+v", v)
	}

	other := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x99}, 48)}}
	if v := (Impl{}).Check(other, regInfo, time.Now().Unix(), trustedApprovers); v.Pass {
		t.Fatal("a different measurement passed")
	}

	future := expires.Add(time.Hour).Unix()
	if v := (Impl{}).Check(values, regInfo, future, trustedApprovers); v.Pass {
		t.Fatal("a time past expiry passed")
	}

	if v := (Impl{}).Check(values, regInfo, time.Now().Unix(), []string{"0x0000000000000000000000000000000000000001"}); v.Pass {
		t.Fatal("a approver outside the trusted list passed")
	}
}

// Clock uses the registration block's time if present, otherwise this machine's clock.
func TestClock_PrefersChainTime(t *testing.T) {
	if got := Clock(l.Registration{ChainTime: 1_700_000_000}); got != 1_700_000_000 {
		t.Fatalf("Clock = %d, want the chain time", got)
	}
	before := time.Now().Unix()
	got := Clock(l.Registration{})
	if got < before || got > before+5 {
		t.Fatalf("Clock without ChainTime = %d, want roughly now (%d)", got, before)
	}
}

// Expiry is judged on the chain clock: however slow the verifier's clock is, if the lookup block's time is past expiry the approval is dead.
func TestCheck_ExpiryUsesChainClockNotLocal(t *testing.T) {
	reg := l.Registration{Expires: 1_000, ChainTime: 2_000, Approver: "0x0000000000000000000000000000000000000001",
		Values: p.Measurements{{Name: "measurementHash", Value: make([]byte, 32)}}}
	// Even if the local clock is stuck in the past (500), Clock(reg) returns 2000 and Check treats the registration as expired.
	v := (Impl{}).Check(p.Measurements{{Name: "PCR0", Value: []byte{1}}}, reg, Clock(reg), []string{reg.Approver})
	found := false
	for _, r := range v.Reason {
		if r == "registration expired" {
			found = true
		}
	}
	if v.Pass || !found {
		t.Fatalf("chain time 2000 > expiry 1000 but not treated as expired: %+v", v)
	}
}
