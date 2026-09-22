// Package ether 는 이더리움(EVM) 체인을 읽기만 한다. chain/register/auditor/ether 가 낸 abigen 바인딩
// (registry.go)을 그대로 쓴다 — 같은 컨트랙트를 두 번 컴파일해 둘 이유가 없다.
package ether

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

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

// defaultLogRangeChunk — Lookup 이 이벤트 로그를 한 번에 물어보는 블록 범위. 공개 RPC(Infura·Alchemy·
// publicnode 류)는 대개 eth_getLogs 의 fromBlock~toBlock 범위를 수천~수만 블록으로 제한한다. 등록된 지
// 오래된 컨트랙트(예: 이 저장소가 올려 본 Sepolia 컨트랙트는 이미 1100만+ 블록째다)를 블록 0부터 한 번에
// 훑으면 그 제한에 걸려 거절당한다 — 그래서 최신 블록에서부터 이 범위씩 거슬러 올라가며 나눠 묻는다.
const defaultLogRangeChunk = 4000

// Backend — 조회에 필요한 것. *ethclient.Client 와 *simulated.Backend.Client() 가 둘 다 만족한다.
// BlockNumber 가 필요한 건 Lookup 이 이벤트 로그를 최신 블록에서부터 나눠 훑기 때문이다.
type Backend interface {
	bind.ContractBackend
	BlockNumber(ctx context.Context) (uint64, error)
}

// Reader — RPC 로 컨트랙트를 읽는다. Backend 가 있으면 그걸 쓰고(시뮬레이션 체인 시험용), 없으면 RPC 에 연결한다.
type Reader struct {
	RPC      string
	Contract common.Address
	Backend  Backend
	// LogRangeChunk — 0 이면 defaultLogRangeChunk. 시험에서 여러 번 나눠 훑는 경로를 실제로 돌려보려고 열어 둠.
	LogRangeChunk uint64
}

var _ l.Reader = (*Reader)(nil)

func (r Reader) client(ctx context.Context) (*regether.Registry, Backend, func(), error) {
	backend := r.Backend
	closeFn := func() {}
	if backend == nil {
		c, err := ethclient.DialContext(ctx, r.RPC)
		if err != nil {
			return nil, nil, nil, err
		}
		backend, closeFn = c, c.Close
	}
	registry, err := regether.NewRegistry(r.Contract, backend)
	if err != nil {
		closeFn()
		return nil, nil, nil, err
	}
	return registry, backend, closeFn, nil
}

// FindAddress — 측정값을 해시해 DefaultPlatform 아래서 등록된 key 를 찾는다. Auditor · Vendor 가 각자
// 등록했으면 둘 다 돌아온다. 등록이 없으면 빈 슬라이스.
func (r Reader) FindAddress(ctx context.Context, values p.Measurements) ([]string, error) {
	h, err := reg.MeasurementHash(values)
	if err != nil {
		return nil, err
	}
	registry, _, done, err := r.client(ctx)
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

// parseKey — address 는 FindAddress 가 준 것과 같은 모양(0x + 32 바이트 hex)이어야 한다. 길이만 보면
// 잘못된 hex(오타·복붙 깨짐)가 common.HexToHash 에서 조용히 0 값으로 바뀌어 "등록 없음"으로 오인될 수
// 있어서, hex 로 실제로 디코드되는지까지 본다.
func parseKey(address string) (common.Hash, error) {
	s := strings.TrimPrefix(address, "0x")
	if len(s) != 64 {
		return common.Hash{}, fmt.Errorf("key %q: want a 32-byte hex value (0x… , 66 글자)", address)
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return common.Hash{}, fmt.Errorf("key %q: not valid hex: %w", address, err)
	}
	return common.BytesToHash(b), nil
}

// Lookup — key(FindAddress 가 준 것)로 등록 정보를 읽는다. 컨트랙트 저장소(records)엔 서명자 · 만료 · 등록
// 시각만 있어서, 메타데이터(서비스 · 버전 · 플랫폼) · 서명은 Registered 이벤트 로그에서 채운다.
//
// Values 에는 원래 PCR 이름별 값이 아니라 측정값 해시 하나만 들어간다 — 컨트랙트가 저장하는 게 그 해시뿐이라
// 개별 PCR 을 되돌릴 방법이 없다. 대조(M2)는 이 해시를 빌드 시점에 같은 방식으로 계산한 해시와 맞대봐야 한다.
func (r Reader) Lookup(ctx context.Context, address string) (l.Registration, error) {
	key, err := parseKey(address)
	if err != nil {
		return l.Registration{}, err
	}
	registry, backend, done, err := r.client(ctx)
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
	ev, err := findRegisteredEvent(ctx, backend, registry, key, r.LogRangeChunk)
	if err != nil {
		return l.Registration{}, fmt.Errorf("records 에는 있지만 이벤트 로그를 못 읽음: %w", err)
	}
	if ev == nil {
		return l.Registration{}, fmt.Errorf("key %s: 등록은 있지만 이벤트 로그가 없다 (오래된 노드가 로그를 지웠을 수 있다)", address)
	}
	return l.Registration{
		Address:   address,
		Values:    p.Measurements{{Name: "measurementHash", Value: ev.Measurement[:]}},
		Meta:      l.Meta{Service: ev.Service, Version: ev.Version, Platform: ev.Platform},
		Expires:   int64(rec.Expires),
		Signer:    rec.Signer.Hex(),
		Signature: ev.Signature,
	}, nil
}

// findRegisteredEvent — Registered 이벤트를 최신 블록에서부터 chunk 블록씩 거슬러 올라가며 찾는다.
// key 하나에 등록은 최대 하나뿐이라(컨트랙트가 중복을 거절한다), 찾으면 바로 돌려준다. 컨트랙트를 배포한
// 지점보다 먼저까지 가면(from == 0 을 다 봤는데도 없으면) 못 찾은 것으로 본다.
func findRegisteredEvent(ctx context.Context, backend Backend, registry *regether.Registry, key common.Hash, chunk uint64) (*regether.RegistryRegistered, error) {
	if chunk == 0 {
		chunk = defaultLogRangeChunk
	}
	latest, err := backend.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	to := latest
	for {
		var from uint64
		if to >= chunk {
			from = to - chunk + 1
		}
		end := to
		iter, err := registry.FilterRegistered(&bind.FilterOpts{Start: from, End: &end, Context: ctx}, [][32]byte{key}, nil)
		if err != nil {
			return nil, err
		}
		found := iter.Next()
		var ev *regether.RegistryRegistered
		if found {
			e := *iter.Event
			ev = &e
		}
		iterErr := iter.Error()
		iter.Close()
		if iterErr != nil {
			return nil, iterErr
		}
		if ev != nil {
			return ev, nil
		}
		if from == 0 {
			return nil, nil
		}
		to = from - 1
	}
}
