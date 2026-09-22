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
	registry, done, err := r.client(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	keys, err := registry.KeysOf(&bind.CallOpts{Context: ctx}, DefaultPlatform, h)
	if err != nil {
		return nil, err
	}
	// keysOf 는 순수 계산이라 등록 여부와 상관없이 항상 키를 낸다. 실제로 등록됐는지는 각 key 로 lookup
	// 을 해서 signer 가 채워졌는지로 본다.
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

// Lookup — key(FindAddress 가 준 것)로 등록 정보를 읽는다. 자세한 이유는 auditor/ether 의 주석 참고 —
// 컨트랙트 저장소엔 서명자 · 만료 · 등록 시각만 있어서 나머지는 이벤트 로그에서 채운다.
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
