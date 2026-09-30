// 시뮬레이션 체인에 register 로 등록해 두고, Reader 로 조회한다. User 는 등록을 안 해서 register/user 가
// 없다 — auditor 패키지로 등록해 두고, user 의 Reader(= auditor 바인딩을 빌려 씀)로 읽는다. chain() ·
// newKey() 는 auditor 쪽 reader_test.go 에도 있는 것과 같은 모양이지만, 여긴 다른 패키지라 다시 적는다.
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

	al "tb/internal/chain/lookup/auditor"
	aud "tb/internal/chain/register/auditor"
	audether "tb/internal/chain/register/auditor/ether"
	ap "tb/internal/tee/parse/auditor"
	up "tb/internal/tee/parse/user"
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
	auditorKey, auditorAddr := newKey(t)
	client := chain(t, deployer, auditorAddr).Client()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := audether.Deploy(ctx, "", client, deployerKey, auditorAddr)
	if err != nil {
		t.Fatal(err)
	}

	// auditor 쪽 타입으로 등록 — 이 브랜치엔 register/user 가 없다.
	aValues := ap.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{0x11}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{0x22}, 48)},
		{Name: "PCR2", Value: bytes.Repeat([]byte{0x33}, 48)},
	}
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	registrar := audether.Registrar{Backend: client, Contract: contract, SenderKey: auditorKey}
	meta := aud.Meta{Meta: al.Meta{Service: "unit-test", Version: "0.1.0", Platform: DefaultPlatform, Arch: "arm64"}, Expires: expires}
	signed, err := registrar.Sign(aValues, meta, auditorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	// user 쪽 타입으로 같은 측정값을 다시 만들어 — user.Reader 가 필드는 같아도 다른 타입을 받는지 확인.
	uValues := make(up.Measurements, len(aValues))
	for i, m := range aValues {
		uValues[i] = up.Measurement{Name: m.Name, Value: m.Value}
	}

	reader := Reader{Contract: contract, Backend: client}

	keys, err := reader.FindAddress(ctx, uValues)
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
	if got.Signer != auditorAddr.Hex() || got.Meta.Service != "unit-test" || got.Meta.Platform != DefaultPlatform || got.Expires != expires.Unix() {
		t.Fatalf("Lookup = %+v", got)
	}
	// 체인이 어느 아키텍처의 등록인지 말한다 — User 는 arch 를 몰라도 찾고, 찾은 뒤에 읽는다.
	if got.Meta.Arch != "arm64" {
		t.Fatalf("Lookup arch = %q, want arm64", got.Meta.Arch)
	}
	h, _ := measurementHash(uValues)
	if !bytes.Equal(got.Values.Get("measurementHash"), h[:]) {
		t.Error("measurement hash 가 등록 때 계산한 것과 다르다")
	}
	// register 패키지의 계산과도 같아야 한다 — 등록은 그쪽으로, 조회는 이쪽으로 했으니.
	ah, _ := aud.MeasurementHash(aValues)
	if h != ah {
		t.Error("user 의 measurementHash 가 register 의 MeasurementHash 와 다르다")
	}

	// 등록되지 않은 값 — 빈 슬라이스.
	other := up.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x99}, 48)}}
	if got, err := reader.FindAddress(ctx, other); err != nil || len(got) != 0 {
		t.Fatalf("등록 안 된 측정값의 FindAddress = %v, %v", got, err)
	}

	// 등록되지 않은 key — 에러.
	if _, err := reader.Lookup(ctx, common.HexToHash("0x3039").Hex()); err == nil {
		t.Fatal("등록되지 않은 key 는 에러여야 한다")
	}

	// 길이는 맞는데 hex 가 아닌 key — 조용히 0 으로 바뀌어 "not registered" 로 둔갑하면 안 되고, 그 자체로
	// 에러여야 한다.
	bad := "0x" + strings.Repeat("zz", 32)
	if _, err := reader.Lookup(ctx, bad); err == nil {
		t.Fatal("hex 가 아닌 key 는 에러여야 한다 (0 으로 조용히 바뀌면 안 된다)")
	}
}

// TestReader_Lookup_ScansMultipleLogChunks — 등록 뒤에도 블록이 계속 쌓이는 실제 체인을 흉내 낸다.
// LogRangeChunk 를 아주 작게 줘서, 최신 블록에서 한 번에 못 찾고 여러 번 거슬러 올라가야만 이벤트를
// 찾는 경로를 실제로 돌려본다.
func TestReader_Lookup_ScansMultipleLogChunks(t *testing.T) {
	deployerKey, deployer := newKey(t)
	auditorKey, auditorAddr := newKey(t)
	sim := chain(t, deployer, auditorAddr)
	client := sim.Client()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := audether.Deploy(ctx, "", client, deployerKey, auditorAddr)
	if err != nil {
		t.Fatal(err)
	}
	aValues := ap.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x44}, 48)}}
	registrar := audether.Registrar{Backend: client, Contract: contract, SenderKey: auditorKey}
	meta := aud.Meta{Meta: al.Meta{Service: "chunk-test", Version: "0.1.0", Platform: DefaultPlatform, Arch: "amd64"}, Expires: time.Now().Add(time.Hour)}
	signed, err := registrar.Sign(aValues, meta, auditorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 8; i++ {
		sim.Commit()
	}

	reader := Reader{Contract: contract, Backend: client, LogRangeChunk: 1}
	got, err := reader.Lookup(ctx, receipt.Key)
	if err != nil {
		t.Fatalf("청크 1개씩 거슬러 올라가며 찾기 실패: %v", err)
	}
	if got.Meta.Service != "chunk-test" {
		t.Fatalf("Lookup = %+v", got)
	}
}
