// Package ether only reads an Ethereum (EVM) chain. It uses the abigen binding (registry.go) produced by
// chain/register/auditor/ether as is; there is no reason to compile the same contract twice.
package ether

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	l "attestamp/internal/chain/lookup/auditor"
	reg "attestamp/internal/chain/register/auditor"
	regether "attestamp/internal/chain/register/auditor/ether"
	p "attestamp/internal/tee/parse/auditor"
)

// DefaultPlatform is the only measurement format supported for now. Once registrations start mixing platforms,
// FindRecordKeys's signature itself will have to grow (the record key in Registry.sol differs per platform).
const DefaultPlatform = "aws-nitro"

// defaultLogRangeChunk is the block range Lookup asks for in one event-log query. Public RPCs (Infura, Alchemy,
// publicnode and the like) usually cap the fromBlock..toBlock range of eth_getLogs at a few thousand to tens of thousands
// of blocks. A contract registered long ago (a Sepolia contract is already past block 11 million) would be rejected if
// scanned from block 0 in one go, so the scan works backwards from the latest block in chunks of this size.
const defaultLogRangeChunk = 4000

// Backend is what a lookup needs. Both *ethclient.Client and *simulated.Backend.Client() satisfy it.
// BlockNumber is needed because Lookup scans the event logs in chunks starting from the latest block.
type Backend interface {
	bind.ContractBackend
	BlockNumber(ctx context.Context) (uint64, error)
}

// Reader reads the contract over RPC. It uses Backend if set (for tests on a simulated chain), otherwise connects to RPC.
type Reader struct {
	RPC          string
	RegistryAddr common.Address
	Backend      Backend
	// LogRangeChunk: if 0, defaultLogRangeChunk. Exposed so tests can exercise the multi-chunk scan path.
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

// FindRecordKeys hashes the measurements and finds the record key registered under DefaultPlatform. A contract has one approver,
// so there is at most one record key. With no registration it returns an empty slice. (If an auditor and a vendor both
// register, that is two contracts, each looked up separately.)
func (r Reader) FindRecordKeys(ctx context.Context, values p.Measurements) ([]string, error) {
	h, err := reg.MeasurementHash(values)
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

// parseRecordKey: recordKey must have the same shape FindRecordKeys returns (0x + 32 bytes of hex). Checking only the length
// would let invalid hex (a typo or a broken paste) silently become the zero value in common.HexToHash and be mistaken for
// "not registered", so it also checks that the string really decodes as hex.
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

// Lookup reads the registration for a record key (as returned by FindRecordKeys). The contract storage (registrations) holds only the
// approver, expiry and registration time, so the metadata (service, version, platform) and the signature are filled in
// from the Registered event log.
//
// Values holds a single measurement hash rather than per-PCR values: the contract stores only that hash, so there is
// no way to recover the individual PCRs. The comparison (compare.Check) must check this hash against one computed the
// same way at build time.
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

// TrustedApprovers is the approver address (APPROVER) fixed in the contract. compare.Check uses it to see whether the
// registration approver is this address.
// It never changes after deployment (APPROVER in Registry.sol is immutable), so fetching it again always gives the same value.
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
// A record key has at most one registration (the contract rejects duplicates), so it returns as soon as it finds one. If the
// scan reaches before the contract's deployment (from == 0 has been scanned and nothing found), it counts as not found.
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
