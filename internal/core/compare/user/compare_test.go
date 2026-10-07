// Compare 는 순수 함수라 그대로 테스트하고, Check 는 실제로 시뮬레이션 체인에 register 해 둔 걸
// chain/lookup 으로 읽어서 맞대본다. User 는 등록을 안 해서 register/user 가 없다 — auditor 패키지로
// 등록해 두고 user 의 Reader(= auditor 바인딩을 빌려 씀)로 읽는다.
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

	al "tb/internal/chain/lookup/auditor"
	l "tb/internal/chain/lookup/user"
	lether "tb/internal/chain/lookup/user/ether"
	aud "tb/internal/chain/register/auditor"
	audether "tb/internal/chain/register/auditor/ether"
	ap "tb/internal/tee/parse/auditor"
	p "tb/internal/tee/parse/user"
)

func TestCompare_ParsedVsBuild(t *testing.T) {
	parsed := p.Measurements{{Name: "PCR0", Value: []byte{1, 2, 3}}}
	build := p.Measurements{{Name: "PCR0", Value: []byte{1, 2, 3}}}
	v := (Impl{}).Compare(parsed, build)
	if !v.Pass {
		t.Fatalf("같은 값인데 불일치: %+v", v)
	}

	tampered := p.Measurements{{Name: "PCR0", Value: []byte{9, 9, 9}}}
	v = (Impl{}).Compare(parsed, tampered)
	if v.Pass {
		t.Fatal("값이 다른데 일치로 나옴")
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

// User — 흐름의 마지막 확인: 실제 배포된 서비스에서 새로 받은 RA 값을 체인 등록 정보와 대조한다.
func TestCheck_AgainstRealChainRegistration(t *testing.T) {
	deployerKey, deployer := newKey(t)
	auditorKey, auditorAddr := newKey(t)
	client := chain(t, deployer, auditorAddr).Client()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := audether.Deploy(ctx, "", client, deployerKey, auditorAddr)
	if err != nil {
		t.Fatal(err)
	}
	aValues := ap.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{0x11}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{0x22}, 48)},
	}
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	registrar := audether.Registrar{Backend: client, Contract: contract, SenderKey: auditorKey}
	meta := aud.Meta{Meta: al.Meta{Service: "svc", Version: "1.0.0", Platform: lether.DefaultPlatform, Arch: "amd64"}, Expires: expires}
	signed, err := registrar.Sign(aValues, meta, auditorKey)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registrar.Register(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}

	// User 는 자기 타입으로 방금 파싱했다고 치고 — 같은 값을 user.Measurements 로 다시 만든다.
	values := make(p.Measurements, len(aValues))
	for i, m := range aValues {
		values[i] = p.Measurement{Name: m.Name, Value: m.Value}
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

	if regInfo.ChainTime <= 0 {
		t.Fatalf("Lookup 이 ChainTime 을 안 채웠다: %+v", regInfo)
	}
	// 실제 호출 경로: now 는 로컬 시계가 아니라 조회 블록의 시각.
	if v := (Impl{}).Check(values, regInfo, Clock(regInfo), trusted); !v.Pass {
		t.Fatalf("등록한 값 그대로인데 실패: %+v", v)
	}

	other := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x99}, 48)}}
	if v := (Impl{}).Check(other, regInfo, time.Now().Unix(), trusted); v.Pass {
		t.Fatal("다른 측정값인데 통과함")
	}

	future := expires.Add(time.Hour).Unix()
	if v := (Impl{}).Check(values, regInfo, future, trusted); v.Pass {
		t.Fatal("만료 지난 시각인데 통과함")
	}

	if v := (Impl{}).Check(values, regInfo, time.Now().Unix(), []string{"0x0000000000000000000000000000000000000001"}); v.Pass {
		t.Fatal("신뢰 목록에 없는데 통과함")
	}
}

// Clock — 등록을 읽어 온 블록의 시각이 있으면 그것, 없으면 이 기계의 시계.
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

// 만료는 체인 시계로 판정한다 — 검증자 시계가 아무리 느려도, 조회 블록 시각이 만료를 지났으면 죽은 승인이다.
func TestCheck_ExpiryUsesChainClockNotLocal(t *testing.T) {
	reg := l.Registration{Expires: 1_000, ChainTime: 2_000, Signer: "0x0000000000000000000000000000000000000001",
		Values: p.Measurements{{Name: "measurementHash", Value: make([]byte, 32)}}}
	// 로컬 시계가 과거(500)에 멈춰 있어도 Clock(reg) 는 2000 을 돌려주고 Check 는 만료로 본다.
	v := (Impl{}).Check(p.Measurements{{Name: "PCR0", Value: []byte{1}}}, reg, Clock(reg), []string{reg.Signer})
	found := false
	for _, r := range v.Reason {
		if r == "registration expired" {
			found = true
		}
	}
	if v.Pass || !found {
		t.Fatalf("체인 시각 2000 > 만료 1000 인데 만료로 안 봄: %+v", v)
	}
}
