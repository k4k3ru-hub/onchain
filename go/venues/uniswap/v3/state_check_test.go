package v3

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
	"math/big"
	"testing"
)

func TestCheckStateStableInputsAndReorg(t *testing.T) {
	c, f := newTestCache(t)
	c.active = true
	if _, err := c.QuotePair(context.Background(), big.NewInt(1000), true); err != nil {
		t.Fatal(err)
	}
	a, err := c.CheckState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.CheckState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Key == "" || a.Key != b.Key || a.Position == 0 || b.CheckedAt.Before(a.CheckedAt) {
		t.Fatal("unchanged inputs not reusable")
	}
	f.reorg = true
	if _, err = c.CheckState(context.Background()); err == nil {
		t.Fatal("reorg accepted")
	}
	if c.snapshot != nil {
		t.Fatal("invalid snapshot retained")
	}
}
func TestCheckStateRejectsConcurrentInvalidation(t *testing.T) {
	c, f := newTestCache(t)
	f.hook = func() { c.mu.Lock(); c.generation++; c.mu.Unlock() }
	if _, err := c.CheckState(context.Background()); err == nil {
		t.Fatal("concurrent invalidation accepted")
	}
}

type advancingCheckRPC struct {
	*stateFake
	head uint64
	fork bool
}

// LatestHeader returns the test chain tip.
//
// Version:
//   - 2026-09-09: Added.
func (f *advancingCheckRPC) LatestHeader(ctx context.Context) (evm.BlockHeader, error) {
	return f.HeaderByNumber(ctx, f.head)
}

// HeaderByNumber returns a canonical header in the selected test fork.
//
// Version:
//   - 2026-09-09: Added.
func (f *advancingCheckRPC) HeaderByNumber(_ context.Context, n uint64) (evm.BlockHeader, error) {
	hash := n
	if f.fork {
		hash += 1000
	}
	return evm.BlockHeader{Number: n, Hash: common.BigToHash(new(big.Int).SetUint64(hash))}, nil
}
func TestCheckStateSeparatesHeadProgressFromChangedInputs(t *testing.T) {
	c, f := newTestCache(t)
	rpc := &advancingCheckRPC{stateFake: f, head: 100}
	c.rpc = rpc
	c.active = true
	ctx := context.Background()
	if _, err := c.QuotePair(ctx, big.NewInt(1000), true); err != nil {
		t.Fatal(err)
	}
	a, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rpc.head++
	b, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Key != b.Key || b.Position != 101 {
		t.Fatal("unchanged inputs changed with head")
	}
	f.crossed = true
	changed, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Key == b.Key {
		t.Fatal("tick bitmap change ignored")
	}
	rpc.head++
	rpc.fork = true
	reorg, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reorg.Key == changed.Key {
		t.Fatal("reorg reused previous quote provenance")
	}
}
