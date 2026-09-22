// Compare 는 순수 함수라 그대로 테스트하고, Check 는 실제로 시뮬레이션 체인에 register 해 둔 걸
// chain/lookup 으로 읽어서 맞대본다 — M2 가 실제로 쓰이는 모양(등록 뒤 조회값과 대조) 그대로.
package auditor

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

	lether "tb/internal/chain/lookup/auditor/ether"
	l "tb/internal/chain/lookup/auditor"
	reg "tb/internal/chain/register/auditor"
	regether "tb/internal/chain/register/auditor/ether"
	p "tb/internal/tee/parse/auditor"
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

func TestCheck_AgainstRealChainRegistration(t *testing.T) {
	deployerKey, deployer := newKey(t)
	auditorKey, auditorAddr := newKey(t)
	_, vendorAddr := newKey(t)
	client := chain(t, deployer, auditorAddr).Client()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contract, _, err := regether.Deploy(ctx, "", client, deployerKey, auditorAddr, vendorAddr)
	if err != nil {
		t.Fatal(err)
	}
	values := p.Measurements{
		{Name: "PCR0", Value: bytes.Repeat([]byte{0x11}, 48)},
		{Name: "PCR1", Value: bytes.Repeat([]byte{0x22}, 48)},
	}
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	registrar := regether.Registrar{Backend: client, Contract: contract, SenderKey: auditorKey}
	meta := reg.Meta{Meta: l.Meta{Service: "svc", Version: "1.0.0", Platform: lether.DefaultPlatform}, Expires: expires}
	signed, err := registrar.Sign(values, meta, auditorKey)
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

	// 방금 등록한 그 값으로 Check — 통과해야 한다.
	if v := (Impl{}).Check(values, regInfo, time.Now().Unix(), trusted); !v.Pass {
		t.Fatalf("등록한 값 그대로인데 실패: %+v", v)
	}

	// 다른 값으로 Check — 해시가 달라서 실패해야 한다.
	other := p.Measurements{{Name: "PCR0", Value: bytes.Repeat([]byte{0x99}, 48)}}
	if v := (Impl{}).Check(other, regInfo, time.Now().Unix(), trusted); v.Pass {
		t.Fatal("다른 측정값인데 통과함")
	}

	// 만료 지난 시각으로 Check — 실패해야 한다.
	future := expires.Add(time.Hour).Unix()
	if v := (Impl{}).Check(values, regInfo, future, trusted); v.Pass {
		t.Fatal("만료 지난 시각인데 통과함")
	}

	// 신뢰 목록에 없는 서명자 — 실패해야 한다.
	if v := (Impl{}).Check(values, regInfo, time.Now().Unix(), []string{"0x0000000000000000000000000000000000000001"}); v.Pass {
		t.Fatal("신뢰 목록에 없는데 통과함")
	}
}
