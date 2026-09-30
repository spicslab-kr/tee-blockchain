// go-ethereum 의 시뮬레이션 체인에 컨트랙트를 배포하고, Auditor 와 Vendor 가 각자 패키지로 등록하고 읽어 본다.
// 컨트랙트 하나 = 서명자 하나라, 둘이 다 등록하는 흐름은 컨트랙트가 둘이다.
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

	// 각 역할이 자기 주소로 자기 컨트랙트를 배포한다.
	aContract, _, err := audether.Deploy(ctx, "", client, deployerKey, auditorAddr)
	if err != nil {
		t.Fatal(err)
	}
	vContract, _, err := venether.Deploy(ctx, "", client, deployerKey, vendorAddr)
	if err != nil {
		t.Fatal(err)
	}
	if aContract == vContract {
		t.Fatal("two deployments must land at different addresses")
	}
	// 조회는 C2 의 일이다. 여기서는 등록이 실제로 들어갔는지 보기 위해 바인딩으로 직접 읽는다.
	aRegistry, err := audether.NewRegistry(aContract, client)
	if err != nil {
		t.Fatal(err)
	}
	vRegistry, err := venether.NewRegistry(vContract, client)
	if err != nil {
		t.Fatal(err)
	}
	call := &bind.CallOpts{Context: ctx}
	if a, err := aRegistry.APPROVER(call); err != nil || a != auditorAddr {
		t.Fatalf("auditor contract APPROVER = %s (%v)", a.Hex(), err)
	}
	if v, err := vRegistry.APPROVER(call); err != nil || v != vendorAddr {
		t.Fatalf("vendor contract APPROVER = %s (%v)", v.Hex(), err)
	}

	expires := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	vValues, aValues := pcrs(0x10)

	// Auditor — auditor 패키지로, 자기 컨트랙트에
	auditor := audether.Registrar{Backend: client, Contract: aContract, SenderKey: auditorKey}
	aMeta := aud.Meta{Meta: al.Meta{Service: "my-app", Version: "1.0.0", Platform: "aws-nitro", Arch: "amd64"}, Expires: expires}
	aSigned, err := auditor.Sign(aValues, aMeta, auditorKey)
	if err != nil {
		t.Fatal(err)
	}
	if aSigned.Signer != auditorAddr.Hex() {
		t.Fatalf("signer %s", aSigned.Signer)
	}
	h, _ := aud.MeasurementHash(aValues)
	onchainEntry := audether.RegistryEntry{Platform: aMeta.Platform, Arch: aMeta.Arch, Service: aMeta.Service, Version: aMeta.Version, Measurement: h, Expires: uint64(expires.Unix())}
	onchainDigest, err := aRegistry.Digest(call, onchainEntry)
	if err != nil || !bytes.Equal(onchainDigest[:], aSigned.Digest) {
		t.Fatalf("Go digest differs from the contract's: %x vs %x (%v)", aSigned.Digest, onchainDigest, err)
	}
	// arch 가 다르면 컨트랙트의 digest 도 달라야 한다 — 같다면 arch 가 서명에 안 들어간 것.
	otherArch := onchainEntry
	otherArch.Arch = "arm64"
	if d, err := aRegistry.Digest(call, otherArch); err != nil || d == onchainDigest {
		t.Fatalf("the contract's digest must depend on arch (%v)", err)
	}
	// arch 가 빈 Entry — 컨트랙트가 MissingField("arch") 로 거부한다. Go 쪽은 Meta.Validate 가 먼저 막으니
	// 바인딩으로 직접 보낸다. 필드 검사는 서명 복원보다 앞이라 서명은 아무거나 된다.
	if field := missingField(t, ctx, client, aRegistry, auditorKey, audether.RegistryEntry{Platform: "aws-nitro", Service: "my-app", Version: "1.0.0", Measurement: h, Expires: uint64(expires.Unix())}, aSigned.Signature); field != "arch" {
		t.Fatalf("an entry without an arch must be rejected with MissingField(\"arch\"), got %q", field)
	}
	aReceipt, err := auditor.Register(ctx, aSigned)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := aRegistry.Lookup(call, common.HexToHash(aReceipt.Key))
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

	// Vendor — Vendor 패키지로, 같은 측정값을 자기 컨트랙트에. 서명자가 달라 키도 다르다
	vendor := venether.Registrar{Backend: client, Contract: vContract, SenderKey: vendorKey}
	vMeta := ven.Meta{Meta: vl.Meta{Service: "my-app", Version: "1.0.0", Platform: "aws-nitro", Arch: "arm64"}, Expires: expires}
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
	// keyFor — 각 컨트랙트가 자기 서명자 기준으로 내는 키가 영수증의 키와 같아야 한다
	if k, err := aRegistry.KeyFor(call, "aws-nitro", h); err != nil || common.Hash(k).Hex() != aReceipt.Key {
		t.Fatalf("auditor keyFor = %x (%v), want %s", k, err, aReceipt.Key)
	}
	if k, err := vRegistry.KeyFor(call, "aws-nitro", h); err != nil || common.Hash(k).Hex() != vReceipt.Key {
		t.Fatalf("vendor keyFor = %x (%v), want %s", k, err, vReceipt.Key)
	}

	// 서명자가 다른 컨트랙트에는 못 올린다 — Auditor 서명을 Vendor 컨트랙트에
	crossValues, _ := pcrs(0x50)
	crossSigned, err := vendor.Sign(crossValues, vMeta, auditorKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vendor.Register(ctx, crossSigned); !errors.Is(err, ven.ErrNotAnApprover) {
		t.Fatalf("auditor signature on the vendor contract: want ErrNotAnApprover, got %v", err)
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
	empty, err := vRegistry.Lookup(call, common.HexToHash("0x3039"))
	if err != nil || empty.Signer != (common.Address{}) || empty.Expires != 0 {
		t.Fatalf("unknown key must be empty: %+v %v", empty, err)
	}

	// 아무나 트랜잭션을 보내도 서명자가 등록자다
	relay := venether.Registrar{Backend: client, Contract: vContract, SenderKey: deployerKey}
	rValues, _ := pcrs(0x40)
	rSigned, err := relay.Sign(rValues, vMeta, vendorKey)
	if err != nil {
		t.Fatal(err)
	}
	rReceipt, err := relay.Register(ctx, rSigned)
	if err != nil {
		t.Fatal(err)
	}
	if rec, _ := vRegistry.Lookup(call, common.HexToHash(rReceipt.Key)); rec.Signer != vendorAddr {
		t.Fatalf("signer must be the vendor, not the sender: %s", rec.Signer.Hex())
	}

	// Registered 이벤트가 arch 를 싣는다 — 조회(chain/lookup)는 여기서 읽는다.
	iter, err := vRegistry.FilterRegistered(&bind.FilterOpts{Context: ctx}, [][32]byte{common.HexToHash(rReceipt.Key)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer iter.Close()
	if !iter.Next() {
		t.Fatalf("no Registered event for %s (%v)", rReceipt.Key, iter.Error())
	}
	if ev := iter.Event; ev.Arch != "arm64" || ev.Service != "my-app" || ev.Signer != vendorAddr {
		t.Fatalf("Registered event = arch %q service %q signer %s", ev.Arch, ev.Service, ev.Signer.Hex())
	}
}

// missingField — Entry 를 바인딩으로 바로 보내서, 컨트랙트가 MissingField 로 거부하면 그 필드 이름을 돌려준다.
// 다른 에러거나 성공이면 실패.
func missingField(t *testing.T, ctx context.Context, client simulated.Client, registry *audether.Registry, senderKey []byte, e audether.RegistryEntry, sig []byte) string {
	t.Helper()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := crypto.ToECDSA(senderKey)
	if err != nil {
		t.Fatal(err)
	}
	opts, err := bind.NewKeyedTransactorWithChainID(sender, chainID)
	if err != nil {
		t.Fatal(err)
	}
	opts.Context = ctx
	_, err = registry.Register(opts, e, sig)
	if err == nil {
		t.Fatal("the contract accepted an entry it must reject")
	}
	var de interface{ ErrorData() interface{} }
	if !errors.As(err, &de) {
		t.Fatalf("no revert data: %v", err)
	}
	data := common.FromHex(de.ErrorData().(string))
	parsed, err := audether.RegistryMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	mf := parsed.Errors["MissingField"]
	if len(data) < 4 || [4]byte(data[:4]) != [4]byte(mf.ID[:4]) {
		t.Fatalf("want MissingField, got revert data %x (%v)", data, err)
	}
	args, err := mf.Unpack(data)
	if err != nil {
		t.Fatal(err)
	}
	field, _ := args.([]any)[0].(string)
	return field
}
