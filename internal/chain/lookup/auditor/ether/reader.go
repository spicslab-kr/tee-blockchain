// Package ether 는 이더리움(EVM) 체인을 읽기만 한다. chain/register/auditor/ether 가 낸 abigen 바인딩
// (registry.go)을 그대로 쓴다 — 같은 컨트랙트를 두 번 컴파일해 둘 이유가 없다.
package ether

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	l "tb/internal/chain/lookup/auditor"
	reg "tb/internal/chain/register/auditor"
	regether "tb/internal/chain/register/auditor/ether"
	p "tb/internal/tee/parse/auditor"
)

// DefaultPlatform — 지금 지원하는 유일한 측정값 형식. 여러 플랫폼을 섞어 등록하기 시작하면 FindAddress 의
// 시그니처 자체를 늘려야 한다 (Registry.sol 의 key 는 platform 마다 다르다).
const DefaultPlatform = "aws-nitro"

// Reader — RPC 로 컨트랙트를 읽는다. Backend 가 있으면 그걸 쓰고(시뮬레이션 체인 시험용), 없으면 RPC 에 연결한다.
type Reader struct {
	RPC      string
	Contract common.Address
	Backend  bind.ContractBackend
}

var _ l.Reader = (*Reader)(nil)

func (r Reader) client(ctx context.Context) (*regether.Registry, func(), error) {
	backend := r.Backend
	closeFn := func() {}
	if backend == nil {
		c, err := ethclient.DialContext(ctx, r.RPC)
		if err != nil {
			return nil, nil, err
		}
		backend, closeFn = c, c.Close
	}
	registry, err := regether.NewRegistry(r.Contract, backend)
	if err != nil {
		closeFn()
		return nil, nil, err
	}
	return registry, closeFn, nil
}

// FindAddress — 측정값을 해시해 DefaultPlatform 아래서 등록된 key 를 찾는다. Auditor · Vendor 가 각자
// 등록했으면 둘 다 돌아온다. 등록이 없으면 빈 슬라이스.
func (r Reader) FindAddress(ctx context.Context, values p.Measurements) ([]string, error) {
	h, err := reg.MeasurementHash(values)
	if err != nil {
		return nil, err
	}
	registry, done, err := r.client(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	keys, err := registry.KeysOf(&bind.CallOpts{Context: ctx}, DefaultPlatform, h)
	if err != nil {
		return nil, err
	}
	// keysOf 는 순수 계산이라 등록 여부와 상관없이 항상 키를 낸다 (키가 0 인 경우도 없다).
	// 실제로 등록됐는지는 각 key 로 lookup 을 해서 signer 가 채워졌는지로 본다.
	var out []string
	for _, key := range [][32]byte{keys.AuditorKey, keys.VendorKey} {
		rec, err := registry.Lookup(&bind.CallOpts{Context: ctx}, key)
		if err != nil {
			return nil, err
		}
		if rec.Signer != (common.Address{}) {
			out = append(out, common.Hash(key).Hex())
		}
	}
	return out, nil
}

// Lookup — key(FindAddress 가 준 것)로 등록 정보를 읽는다. 컨트랙트 저장소(records)엔 서명자 · 만료 · 등록
// 시각만 있어서, 메타데이터(서비스 · 버전 · 플랫폼) · 서명은 Registered 이벤트 로그에서 채운다.
//
// Values 에는 원래 PCR 이름별 값이 아니라 측정값 해시 하나만 들어간다 — 컨트랙트가 저장하는 게 그 해시뿐이라
// 개별 PCR 을 되돌릴 방법이 없다. 대조(M2)는 이 해시를 빌드 시점에 같은 방식으로 계산한 해시와 맞대봐야 한다.
func (r Reader) Lookup(ctx context.Context, address string) (l.Registration, error) {
	if len(address) != 66 {
		return l.Registration{}, fmt.Errorf("key %q: want a 32-byte hex value (0x… , 66 글자)", address)
	}
	key := common.HexToHash(address)
	registry, done, err := r.client(ctx)
	if err != nil {
		return l.Registration{}, err
	}
	defer done()
	rec, err := registry.Lookup(&bind.CallOpts{Context: ctx}, key)
	if err != nil {
		return l.Registration{}, err
	}
	if rec.Signer == (common.Address{}) {
		return l.Registration{}, fmt.Errorf("key %s: not registered", address)
	}
	iter, err := registry.FilterRegistered(&bind.FilterOpts{Start: 0, Context: ctx}, [][32]byte{key}, nil)
	if err != nil {
		return l.Registration{}, fmt.Errorf("records 에는 있지만 이벤트 로그를 못 읽음: %w", err)
	}
	defer iter.Close()
	if !iter.Next() {
		return l.Registration{}, fmt.Errorf("key %s: 등록은 있지만 이벤트 로그가 없다 (오래된 노드가 로그를 지웠을 수 있다)", address)
	}
	ev := iter.Event
	return l.Registration{
		Address:   address,
		Values:    p.Measurements{{Name: "measurementHash", Value: ev.Measurement[:]}},
		Meta:      l.Meta{Service: ev.Service, Version: ev.Version, Platform: ev.Platform},
		Expires:   int64(rec.Expires),
		Signer:    rec.Signer.Hex(),
		Signature: ev.Signature,
	}, nil
}
