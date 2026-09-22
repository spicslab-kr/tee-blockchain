// 시뮬레이션 체인에 register 로 등록해 두고, Reader 로 조회한다. chain() · newKey() 는 auditor 쪽 reader_test.go
// 에도 있는 것과 같은 모양이지만, 여긴 다른 패키지라 그대로 가져다 쓸 수 없어 다시 적는다.
package ether

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
	reg "tb/internal/chain/register/Vendor"
	regether "tb/internal/chain/register/Vendor/ether"
	p "tb/internal/tee/parse/Vendor"
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

func TestReader_FindAddressAndLookup(t *testing.T) {
	deployerKey, deployer := newKey(t)
	_, auditorAddr := newKey(t)
	vendorKey, vendorAddr := newKey(t)
	client := chain(t, deployer, vendorAddr).Client()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := regether.Deploy(ctx, "", client, deployerKey, auditorAddr, vendorAddr)
	if err != nil {
		t.Fatal(err)
	}

	values := p.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{0x11}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{0x22}, 48)},
		{Name: "PCR2", Value: bytes.Repeat([]byte{0x33}, 48)},
	}
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	registrar := regether.Registrar{Backend: client, Contract: contract, SenderKey: vendorKey}
	meta := reg.Meta{Meta: l.Meta{Service: "unit-test", Version: "0.1.0", Platform: DefaultPlatform}, Expires: expires}
	signed, err := registrar.Sign(values, meta, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	reader := Reader{Contract: contract, Backend: client}

	keys, err := reader.FindAddress(ctx, values)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != receipt.Key {
		t.Fatalf("FindAddress = %v, want [%s]", keys, receipt.Key)
	}

	got, err := reader.Lookup(ctx, receipt.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Signer != vendorAddr.Hex() || got.Meta.Service != "unit-test" || got.Meta.Platform != DefaultPlatform || got.Expires != expires.Unix() {
		t.Fatalf("Lookup = %+v", got)
	}
	h, _ := reg.MeasurementHash(values)
	if !bytes.Equal(got.Values.Get("measurementHash"), h[:]) {
		t.Error("measurement hash 가 등록 때 계산한 것과 다르다")
	}

	// 등록되지 않은 값 — 빈 슬라이스.
	other := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x99}, 48)}}
	if got, err := reader.FindAddress(ctx, other); err != nil || len(got) != 0 {
		t.Fatalf("등록 안 된 측정값의 FindAddress = %v, %v", got, err)
	}

	// 등록되지 않은 key — 에러.
	if _, err := reader.Lookup(ctx, common.HexToHash("0x3039").Hex()); err == nil {
		t.Fatal("등록되지 않은 key 는 에러여야 한다")
	}
}
