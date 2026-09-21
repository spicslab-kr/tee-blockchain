// go-ethereum 의 시뮬레이션 체인에 컨트랙트를 배포하고, Auditor 와 Vendor 가 각자 패키지로 등록하고 읽어 본다.
// 실제 노드가 없어도 돈다. 실제 체인(Sepolia)에서 한 기록은 setup/*/chain.md 에 있다.
package register_test

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"

	vl "tb/internal/chain/lookup/Vendor"
	al "tb/internal/chain/lookup/auditor"
	ven "tb/internal/chain/register/Vendor"
	venether "tb/internal/chain/register/Vendor/ether"
	aud "tb/internal/chain/register/auditor"
	audether "tb/internal/chain/register/auditor/ether"
	vp "tb/internal/tee/parse/Vendor"
	ap "tb/internal/tee/parse/auditor"
)

func newKey(t *testing.T) ([]byte, common.Address) {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return crypto.FromECDSA(k), crypto.PubkeyToAddress(k.PublicKey)
}

// chain — 시뮬레이션 체인. 트랜잭션이 채굴되도록 뒤에서 계속 블록을 만든다.
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

func pcrs(seed byte) (vp.Measurements, ap.Measurements) {
	v := vp.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{seed}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{seed + 1}, 48)},
		{Name: "PCR2", Value: bytes.Repeat([]byte{seed + 2}, 48)},
	}
	a := make(ap.Measurements, len(v))
	for i, m := range v {
		a[i] = ap.Measurement{Name: m.Name, Value: m.Value}
	}
	return v, a
}

func TestEther_AuditorAndVendorRegister(t *testing.T) {
	deployerKey, deployer := newKey(t)
	auditorKey, auditorAddr := newKey(t)
	vendorKey, vendorAddr := newKey(t)
	sim := chain(t, deployer, auditorAddr, vendorAddr)
	client := sim.Client()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := venether.Deploy(ctx, "", client, deployerKey, auditorAddr, vendorAddr)
	if err != nil {
		t.Fatal(err)
	}
	// 조회는 C2 의 일이다. 여기서는 등록이 실제로 들어갔는지 보기 위해 바인딩으로 직접 읽는다.
	registry, err := venether.NewRegistry(contract, client)
	if err != nil {
		t.Fatal(err)
	}
	call := &bind.CallOpts{Context: ctx}

	expires := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	vValues, aValues := pcrs(0x10)

	// Auditor — auditor 패키지로
	auditor := audether.Registrar{Backend: client, Contract: contract, SenderKey: auditorKey}
	aMeta := aud.Meta{Meta: al.Meta{Service: "my-app", Version: "1.0.0", Platform: "aws-nitro"}, Expires: expires}
	aSigned, err := auditor.Sign(aValues, aMeta, auditorKey)
	if err != nil {
		t.Fatal(err)
	}
	if aSigned.Signer != auditorAddr.Hex() {
		t.Fatalf("signer %s", aSigned.Signer)
	}
	h, _ := aud.MeasurementHash(aValues)
	onchainDigest, err := registry.Digest(call, venether.RegistryEntry{Platform: aMeta.Platform, Service: aMeta.Service, Version: aMeta.Version, Measurement: h, Expires: uint64(expires.Unix())})
	if err != nil || !bytes.Equal(onchainDigest[:], aSigned.Digest) {
		t.Fatalf("Go digest differs from the contract's: %x vs %x (%v)", aSigned.Digest, onchainDigest, err)
	}
	aReceipt, err := auditor.Register(ctx, aSigned)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := registry.Lookup(call, common.HexToHash(aReceipt.Key))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Signer != auditorAddr || rec.Expires != uint64(expires.Unix()) || rec.RegisteredAt == 0 || rec.ChainTime < rec.RegisteredAt {
		t.Fatalf("lookup: %+v", rec)
	}

	// 같은 것을 다시 — 거부
	if _, err := auditor.Register(ctx, aSigned); !errors.Is(err, aud.ErrAlreadyRegistered) {
		t.Fatalf("want ErrAlreadyRegistered, got %v", err)
	}

	// Vendor — Vendor 패키지로, 같은 측정값. 키가 다르다
	vendor := venether.Registrar{Backend: client, Contract: contract, SenderKey: vendorKey}
	vMeta := ven.Meta{Meta: vl.Meta{Service: "my-app", Version: "1.0.0", Platform: "aws-nitro"}, Expires: expires}
	vSigned, err := vendor.Sign(vValues, vMeta, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	vReceipt, err := vendor.Register(ctx, vSigned)
	if err != nil {
		t.Fatal(err)
	}
	if vReceipt.Key == aReceipt.Key {
		t.Fatal("auditor and vendor registrations must have different keys")
	}
	keys, err := registry.KeysOf(call, "aws-nitro", h)
	if err != nil || common.Hash(keys.AuditorKey).Hex() != aReceipt.Key || common.Hash(keys.VendorKey).Hex() != vReceipt.Key {
		t.Fatalf("keysOf must return both keys: %+v (%v)", keys, err)
	}

	// 제3자 키 — 서명은 되지만 컨트랙트가 거부
	strangerKey, _ := newKey(t)
	sValues, _ := pcrs(0x20)
	sSigned, err := vendor.Sign(sValues, vMeta, strangerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vendor.Register(ctx, sSigned); !errors.Is(err, ven.ErrNotAnApprover) {
		t.Fatalf("want ErrNotAnApprover, got %v", err)
	}

	// 서명 뒤 값을 바꾸면 — 서명자가 다른 주소로 복원되어 거부
	tampered := vSigned
	tampered.Meta.Version = "1.0.1"
	if _, err := vendor.Register(ctx, tampered); !errors.Is(err, ven.ErrNotAnApprover) {
		t.Fatalf("tampered: want ErrNotAnApprover, got %v", err)
	}

	// 이미 지난 만료 — 거부
	past := vMeta
	past.Expires = time.Now().Add(-time.Hour)
	pValues, _ := pcrs(0x30)
	pSigned, err := vendor.Sign(pValues, past, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vendor.Register(ctx, pSigned); !errors.Is(err, ven.ErrExpiryPassed) {
		t.Fatalf("want ErrExpiryPassed, got %v", err)
	}

	// 등록되지 않은 키 — 비어 있음
	empty, err := registry.Lookup(call, common.HexToHash("0x3039"))
	if err != nil || empty.Signer != (common.Address{}) || empty.Expires != 0 {
		t.Fatalf("unknown key must be empty: %+v %v", empty, err)
	}

	// 아무나 트랜잭션을 보내도 서명자가 등록자다
	relay := venether.Registrar{Backend: client, Contract: contract, SenderKey: deployerKey}
	rValues, _ := pcrs(0x40)
	rSigned, err := relay.Sign(rValues, vMeta, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	rReceipt, err := relay.Register(ctx, rSigned)
	if err != nil {
		t.Fatal(err)
	}
	if rec, _ := registry.Lookup(call, common.HexToHash(rReceipt.Key)); rec.Signer != vendorAddr {
		t.Fatalf("signer must be the vendor, not the sender: %s", rec.Signer.Hex())
	}
}
