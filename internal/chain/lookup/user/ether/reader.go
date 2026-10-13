// Package ether only reads an Ethereum (EVM) chain. The user role does not register, so there is no chain/register/user;
// the contract binding (registry.go) is just a stateless client, so the auditor's is borrowed as is. There is no reason to
// compile the contract a third time.
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

	l "attestamp/internal/chain/lookup/user"
	regether "attestamp/internal/chain/register/auditor/ether"
	p "attestamp/internal/tee/parse/user"
)

// DefaultPlatform is the only measurement format supported for now.
const DefaultPlatform = "aws-nitro"

// defaultLogRangeChunk is the block range Lookup asks for in one event-log query. For the reason, see the same constant
// in auditor/ether: public RPCs cap the block range of eth_getLogs, so scanning an old contract from block 0 in one go
// is rejected.
const defaultLogRangeChunk = 4000

// Backend is what a lookup needs. Both *ethclient.Client and *simulated.Backend.Client() satisfy it.
type Backend interface {
	bind.ContractBackend
	BlockNumber(ctx context.Context) (uint64, error)
}

// Reader reads the contract over RPC. It uses Backend if set (for tests on a simulated chain), otherwise connects to RPC.
type Reader struct {
	RPC      string
	Contract common.Address
	Backend  Backend
	// LogRangeChunk: if 0, defaultLogRangeChunk.
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

// measurementHash is the same computation as MeasurementHash in chain/register (keccak256 over "name=value(hex)\n" in name order).
// The user role does not register, so there is no register package and it is written out again here. If the computation changes, change both.
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

// FindAddress hashes the measurements and finds the key registered under DefaultPlatform.
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
	// keyFor is a pure computation, so it always yields a key regardless of registration (and the key is never zero).
	// Whether it is actually registered is seen by calling lookup with that key and checking that signer is filled in.
	rec, err := registry.Lookup(&bind.CallOpts{Context: ctx}, key)
	if err != nil {
		return nil, err
	}
	if rec.Signer == (common.Address{}) {
		return nil, nil
	}
	return []string{common.Hash(key).Hex()}, nil
}

// parseKey: checking only the length would let invalid hex (a typo or a broken paste) silently become the zero value in
// common.HexToHash and be mistaken for "not registered", so it also checks that the string really decodes as hex.
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

// Lookup reads the registration for a key (as returned by FindAddress). For the reason, see the comment in auditor/ether:
// the contract storage holds only the signer, expiry and registration time, so the rest is filled in from the event log.
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

// TrustedSigners is the signer address (APPROVER) fixed in the contract. compare.Check uses it to see whether the
// registration signer is this address.
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

// findRegisteredEvent finds the Registered event by scanning backwards from the latest block, chunk blocks at a time.
// For the reason, see the same function in auditor/ether.
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
