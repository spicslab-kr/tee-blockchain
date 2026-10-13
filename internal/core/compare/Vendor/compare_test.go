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

	l "tb/internal/chain/lookup/Vendor"
	lether "tb/internal/chain/lookup/Vendor/ether"
	reg "tb/internal/chain/register/Vendor"
	regether "tb/internal/chain/register/Vendor/ether"
	p "tb/internal/tee/parse/Vendor"
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

func TestCheck_AgainstRealChainRegistration(t *testing.T) {
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
	}
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	registrar := regether.Registrar{Backend: client, Contract: contract, SenderKey: vendorKey}
	meta := reg.Meta{Meta: l.Meta{Service: "svc", Version: "1.0.0", Platform: lether.DefaultPlatform, Arch: "amd64"}, Expires: expires}
	signed, err := registrar.Sign(values, meta, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	reader := lether.Reader{Contract: contract, Backend: client}
	regInfo, err := reader.Lookup(ctx, receipt.Key)
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := reader.TrustedSigners(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Check with the value just registered; it must pass.
	if v := (Impl{}).Check(values, regInfo, time.Now().Unix(), trusted); !v.Pass {
		t.Fatalf("exactly the registered value failed: %+v", v)
	}

	// Check with a different value; the hash differs, so it must fail.
	other := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x99}, 48)}}
	if v := (Impl{}).Check(other, regInfo, time.Now().Unix(), trusted); v.Pass {
		t.Fatal("a different measurement passed")
	}

	// Check with a time past expiry; it must fail.
	future := expires.Add(time.Hour).Unix()
	if v := (Impl{}).Check(values, regInfo, future, trusted); v.Pass {
		t.Fatal("a time past expiry passed")
	}

	// A signer outside the trusted list; it must fail.
	if v := (Impl{}).Check(values, regInfo, time.Now().Unix(), []string{"0x0000000000000000000000000000000000000001"}); v.Pass {
		t.Fatal("a signer outside the trusted list passed")
	}
}
