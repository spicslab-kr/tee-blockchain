// Package ether reads registrations from an Ethereum (EVM) chain. The user role has no registrar, so it uses the
// abigen binding from chain/register/auditor/ether, so the contract is compiled once.
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

// DefaultPlatform is the platform FindRecordKeys looks up.
const DefaultPlatform = "aws-nitro"

// defaultLogRangeChunk is the block range Lookup asks for in one event-log query. Public RPCs cap the fromBlock..toBlock
// range of eth_getLogs at a few thousand to tens of thousands of blocks, so the scan walks backwards from the latest
// block in chunks of this size.
const defaultLogRangeChunk = 4000

// Backend is what a lookup needs. Both *ethclient.Client and *simulated.Backend.Client() satisfy it.
type Backend interface {
	bind.ContractBackend
	BlockNumber(ctx context.Context) (uint64, error)
}

// Reader reads one Registry contract. It uses Backend if set (tests on a simulated chain), otherwise it connects to RPC.
type Reader struct {
	RPC          string
	RegistryAddr common.Address
	Backend      Backend
	// LogRangeChunk: if 0, defaultLogRangeChunk. Tests lower it to exercise the multi-chunk scan.
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
	registry, err := regether.NewRegistry(r.RegistryAddr, backend)
	if err != nil {
		closeFn()
		return nil, nil, nil, err
	}
	return registry, backend, closeFn, nil
}

// measurementHash is the same computation as MeasurementHash in chain/register/auditor (keccak256 over "name=value(hex)\n"
// in name order), repeated here because the user role has no register package. A change goes into both.
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

// FindRecordKeys hashes the measurements and returns the record key registered for them under DefaultPlatform, or an
// empty result if there is none.
func (r Reader) FindRecordKeys(ctx context.Context, values p.Measurements) ([]string, error) {
	h, err := measurementHash(values)
	if err != nil {
		return nil, err
	}
	registry, _, done, err := r.client(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	recordKey, err := registry.RecordKeyFor(&bind.CallOpts{Context: ctx}, DefaultPlatform, h)
	if err != nil {
		return nil, err
	}
	// recordKeyFor is a pure computation, so it always yields a record key regardless of registration (and it is never zero).
	// Whether it is actually registered is seen by calling lookup with that record key and checking that approver is filled in.
	rec, err := registry.Lookup(&bind.CallOpts{Context: ctx}, recordKey)
	if err != nil {
		return nil, err
	}
	if rec.Approver == (common.Address{}) {
		return nil, nil
	}
	return []string{common.Hash(recordKey).Hex()}, nil
}

// parseRecordKey accepts the form FindRecordKeys returns (0x + 32 bytes of hex). It decodes the hex itself because
// common.HexToHash turns invalid hex into the zero hash, which would read as "not registered".
func parseRecordKey(recordKey string) (common.Hash, error) {
	s := strings.TrimPrefix(recordKey, "0x")
	if len(s) != 64 {
		return common.Hash{}, fmt.Errorf("record key %q: want a 32-byte hex value (0x…, 66 chars)", recordKey)
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return common.Hash{}, fmt.Errorf("record key %q: not valid hex: %w", recordKey, err)
	}
	return common.BytesToHash(b), nil
}

// Lookup reads the registration for a record key (as returned by FindRecordKeys). The contract's registrations mapping
// holds the approver, expiry and registration time; service, version, platform, arch and the signature come from the
// Registered event. Values holds the single measurement hash the contract stores. ChainTime is the time of the block
// that served the lookup.
func (r Reader) Lookup(ctx context.Context, recordKey string) (l.Registration, error) {
	parsedKey, err := parseRecordKey(recordKey)
	if err != nil {
		return l.Registration{}, err
	}
	registry, backend, done, err := r.client(ctx)
	if err != nil {
		return l.Registration{}, err
	}
	defer done()
	rec, err := registry.Lookup(&bind.CallOpts{Context: ctx}, parsedKey)
	if err != nil {
		return l.Registration{}, err
	}
	if rec.Approver == (common.Address{}) {
		return l.Registration{}, fmt.Errorf("record key %s: not registered", recordKey)
	}
	ev, err := findRegisteredEvent(ctx, backend, registry, parsedKey, r.LogRangeChunk)
	if err != nil {
		return l.Registration{}, fmt.Errorf("registration exists but its event log could not be read: %w", err)
	}
	if ev == nil {
		return l.Registration{}, fmt.Errorf("record key %s: registered, but no event log found (an old node may have pruned its logs)", recordKey)
	}
	return l.Registration{
		RecordKey: recordKey,
		Values:    p.Measurements{{Name: "measurementHash", Value: ev.Measurement[:]}},
		Meta:      l.Meta{Service: ev.Service, Version: ev.Version, Platform: ev.Platform, Arch: ev.Arch},
		Expires:   int64(rec.Expires),
		ChainTime: int64(rec.ChainTime),
		Approver:  rec.Approver.Hex(),
		Signature: ev.Signature,
	}, nil
}

// TrustedApprovers returns the registry's APPROVER, which is immutable. compare.Check accepts a registration whose
// approver is in the trusted list.
func (r Reader) TrustedApprovers(ctx context.Context) ([]string, error) {
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
// findRegisteredEvent scans backwards from the latest block, chunk blocks at a time, for the Registered event of a record
// key. The contract rejects duplicates, so the first match is the only one. Reaching block 0 without a match returns nil.
func findRegisteredEvent(ctx context.Context, backend Backend, registry *regether.Registry, recordKey common.Hash, chunk uint64) (*regether.RegistryRegistered, error) {
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
		iter, err := registry.FilterRegistered(&bind.FilterOpts{Start: from, End: &end, Context: ctx}, [][32]byte{recordKey}, nil)
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
