package ether

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	regether "attestamp/internal/chain/register/auditor/ether"
)

// ignoresCtx is a Backend whose log queries never look at their context and never find anything, on a chain whose
// latest block is as far as a uint64 goes. It cancels the test's context at its cancelAt-th query.
type ignoresCtx struct {
	bind.ContractBackend // nil: findRegisteredEvent calls only BlockNumber and FilterLogs
	queries              int
	cancelAt             int
	cancel               context.CancelFunc
}

func (b *ignoresCtx) BlockNumber(context.Context) (uint64, error) { return math.MaxUint64, nil }

func (b *ignoresCtx) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	b.queries++
	if b.queries == b.cancelAt {
		b.cancel()
	}
	return nil, nil
}

// The backwards scan stops when its context ends, even on a backend that does not: here the scan would otherwise
// make 2^64 queries.
func TestFindRegisteredEvent_StopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &ignoresCtx{cancelAt: 3, cancel: cancel}
	registry, err := regether.NewRegistry(common.Address{1}, backend)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		ev  *regether.RegistryRegistered
		err error
	}
	done := make(chan result, 1)
	go func() {
		ev, err := findRegisteredEvent(ctx, backend, registry, common.Hash{2}, 1)
		done <- result{ev, err}
	}()
	select {
	case r := <-done:
		if r.ev != nil || !errors.Is(r.err, context.Canceled) {
			t.Fatalf("findRegisteredEvent = %v, %v; want nil, context.Canceled", r.ev, r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("findRegisteredEvent still scanning 10 s after its context was cancelled")
	}
	if backend.queries != 3 {
		t.Fatalf("%d queries, want 3: the scan must stop at the first step after the context ends", backend.queries)
	}
}
