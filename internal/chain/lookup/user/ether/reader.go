// Package ether 는 이더리움(EVM) 체인을 읽기만 한다. User 는 등록을 안 해서 chain/register/user 가 없다 —
// 컨트랙트 바인딩(registry.go)은 상태가 없는 클라이언트일 뿐이라 auditor 것을 그대로 빌려 쓴다. 컨트랙트를
// 세 번째로 다시 컴파일해 둘 이유가 없다.
package ether

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/crypto/sha3"

	l "tb/internal/chain/lookup/user"
	regether "tb/internal/chain/register/auditor/ether"
	p "tb/internal/tee/parse/user"
)

// DefaultPlatform — 지금 지원하는 유일한 측정값 형식.
const DefaultPlatform = "aws-nitro"

// defaultLogRangeChunk — Lookup 이 이벤트 로그를 한 번에 물어보는 블록 범위. 자세한 이유는 auditor/ether
// 의 같은 상수 주석 참고 — 공개 RPC 는 eth_getLogs 의 블록 범위를 제한해서, 오래된 컨트랙트를 블록 0부터
// 한 번에 훑으면 거절당한다.
const defaultLogRangeChunk = 4000

// Backend — 조회에 필요한 것. *ethclient.Client 와 *simulated.Backend.Client() 가 둘 다 만족한다.
type Backend interface {
	bind.ContractBackend
	BlockNumber(ctx context.Context) (uint64, error)
}

// Reader — RPC 로 컨트랙트를 읽는다. Backend 가 있으면 그걸 쓰고(시뮬레이션 체인 시험용), 없으면 RPC 에 연결한다.
type Reader struct {
	RPC      string
	Contract common.Address
	Backend  Backend
	// LogRangeChunk — 0 이면 defaultLogRangeChunk.
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

// measurementHash — chain/register 의 MeasurementHash 와 같은 계산(이름순 "이름=값(hex)\n" 을 keccak256).
// User 는 등록을 안 하니 register 패키지가 없어서, 여기서만 다시 적는다. 계산 방식을 고치면 양쪽 다 고쳐야 한다.
func measurementHash(values p.Measurements) ([32]byte, error) {
	var out [32]byte
	if len(values) == 0 {
		return out, errors.New("no measurements")
	}
	sorted := make(p.Measurements, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha3.NewLegacyKeccak256()
	for i, m := range sorted {
		if m.Name == "" || len(m.Value) == 0 {
			return out, errors.New("measurement with empty name or value")
		}
		if i > 0 && sorted[i-1].Name == m.Name {
			return out, errors.New("duplicate measurement name: " + m.Name)
		}
		h.Write([]byte(m.Name))
		h.Write([]byte("="))
		h.Write([]byte(hex.EncodeToString(m.Value)))
		h.Write([]byte("\n"))
	}
	copy(out[:], h.Sum(nil))
	return out, nil
}

// FindAddress — 측정값을 해시해 DefaultPlatform 아래서 등록된 key 를 찾는다.
func (r Reader) FindAddress(ctx context.Context, values p.Measurements) ([]string, error) {
	h, err := measurementHash(values)
	if err != nil {
		return nil, err
	}
	registry, _, done, err := r.client(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	key, err := registry.KeyFor(&bind.CallOpts{Context: ctx}, DefaultPlatform, h)
	if err != nil {
		return nil, err
	}
	// keyFor 는 순수 계산이라 등록 여부와 상관없이 항상 키를 낸다 (키가 0 인 경우도 없다).
	// 실제로 등록됐는지는 그 key 로 lookup 을 해서 signer 가 채워졌는지로 본다.
	rec, err := registry.Lookup(&bind.CallOpts{Context: ctx}, key)
	if err != nil {
		return nil, err
	}
	if rec.Signer == (common.Address{}) {
		return nil, nil
	}
	return []string{common.Hash(key).Hex()}, nil
}

// parseKey — 길이만 보면 잘못된 hex(오타·복붙 깨짐)가 common.HexToHash 에서 조용히 0 값으로 바뀌어
// "등록 없음"으로 오인될 수 있어서, hex 로 실제로 디코드되는지까지 본다.
func parseKey(address string) (common.Hash, error) {
	s := strings.TrimPrefix(address, "0x")
	if len(s) != 64 {
		return common.Hash{}, fmt.Errorf("key %q: want a 32-byte hex value (0x…, 66 chars)", address)
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return common.Hash{}, fmt.Errorf("key %q: not valid hex: %w", address, err)
	}
	return common.BytesToHash(b), nil
}

// Lookup — key(FindAddress 가 준 것)로 등록 정보를 읽는다. 자세한 이유는 auditor/ether 의 주석 참고 —
// 컨트랙트 저장소엔 서명자 · 만료 · 등록 시각만 있어서 나머지는 이벤트 로그에서 채운다.
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
		return l.Registration{}, fmt.Errorf("record exists but its event log could not be read: %w", err)
	}
	if ev == nil {
		return l.Registration{}, fmt.Errorf("key %s: registered, but no event log found (an old node may have pruned its logs)", address)
	}
	return l.Registration{
		Address:   address,
		Values:    p.Measurements{{Name: "measurementHash", Value: ev.Measurement[:]}},
		Meta:      l.Meta{Service: ev.Service, Version: ev.Version, Platform: ev.Platform, Arch: ev.Arch},
		Expires:   int64(rec.Expires),
		ChainTime: int64(rec.ChainTime),
		Signer:    rec.Signer.Hex(),
		Signature: ev.Signature,
	}, nil
}

// TrustedSigners — 컨트랙트에 고정된 서명자(APPROVER) 주소. M2 의 Check 가 등록 서명자가 이 주소인지
// 볼 때 쓴다.
func (r Reader) TrustedSigners(ctx context.Context) ([]string, error) {
	registry, _, done, err := r.client(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	a, err := registry.APPROVER(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, err
	}
	return []string{a.Hex()}, nil
}

// findRegisteredEvent — Registered 이벤트를 최신 블록에서부터 chunk 블록씩 거슬러 올라가며 찾는다.
// 자세한 이유는 auditor/ether 의 같은 함수 주석 참고.
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
