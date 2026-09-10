package clmm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	sui "github.com/k4k3ru-hub/onchain/go/sui"
)

type stateReaderFake struct {
	cp      sui.CheckpointSequenceNumber
	obj     *sui.Object
	pages   int
	batches int
	partial bool
	onRead  func()
}

// LatestCheckpoint provides the configured test behavior.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateReaderFake) LatestCheckpoint(context.Context) (sui.Checkpoint, error) {
	return sui.Checkpoint{SequenceNumber: f.cp, Timestamp: time.Now()}, nil
}

// ObjectAtCheckpoint provides the configured test behavior.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateReaderFake) ObjectAtCheckpoint(_ context.Context, _ sui.Address, cp sui.CheckpointSequenceNumber) (*sui.Object, error) {
	if cp != f.cp {
		return nil, fmt.Errorf("wrong checkpoint")
	}
	return f.obj, nil
}

// DynamicValuesAtCheckpoint provides the configured test behavior.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateReaderFake) DynamicValuesAtCheckpoint(_ context.Context, _ sui.Address, cp sui.CheckpointSequenceNumber, _ int, cursor string) (sui.DynamicValuePage, error) {
	if cp != f.cp {
		return sui.DynamicValuePage{}, fmt.Errorf("wrong checkpoint")
	}
	f.pages++
	if f.onRead != nil {
		f.onRead()
	}
	if cursor == "" {
		return sui.DynamicValuePage{Values: []json.RawMessage{json.RawMessage(`{"value":{"index":{"bits":4294967196},"sqrt_price":"9223372036854775808","liquidity_net":{"bits":"1000000"}}}`)}, HasNextPage: !f.partial, NextCursor: "next"}, nil
	}
	return sui.DynamicValuePage{Values: []json.RawMessage{json.RawMessage(`{"value":{"index":{"bits":100},"sqrt_price":"36893488147419103232","liquidity_net":{"bits":"340282366920938463463374607431767211456"}}}`)}}, nil
}
func cacheFixture(t *testing.T) (*StateCache, *stateReaderFake, QuotePairParams) {
	t.Helper()
	a, err := sui.ParseAddress("0x9")
	if err != nil {
		t.Fatal(err)
	}
	f := &stateReaderFake{cp: 123, obj: &sui.Object{Address: a, Version: 10, Move: &sui.MoveObject{Type: "0x1::pool::Pool<0x2::a::A,0x2::b::B>", JSON: json.RawMessage(`{"tick_spacing":"100","coin_a":"1","coin_b":"1","current_sqrt_price":"18446744073709551616","current_tick_index":{"bits":0},"liquidity":"1000000","fee_rate":"500","tick_manager":{"ticks":{"id":"0x8","size":"2"}}}`)}}}
	c, err := NewStateCache(f, a, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return c, f, QuotePairParams{Bid: QuoteExactInputParams{Pool: Pool{Address: a}, AmountIn: 1000, A2B: true}, Ask: QuoteExactOutputParams{Pool: Pool{Address: a}, AmountOut: 998, A2B: false}}
}

// TestStateCacheReuse verifies pinned batches, version-based refresh and concurrent invalidation.
//
// Version:
//   - 2026-09-11: Verify bounded keyed acquisition.
func TestStateCacheReuse(t *testing.T) {
	c, f, p := cacheFixture(t)
	r, err := c.QuotePair(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Checkpoint != 123 || r.Bid.AmountOut != 998 || f.batches != 16 {
		t.Fatalf("%+v pages=%d", r, f.batches)
	}
	f.cp = 124
	r, err = c.QuotePair(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if f.batches != 16 || r.Checkpoint != 124 {
		t.Fatal("failed reuse at confirmed newer checkpoint")
	}
	f.obj.Version++
	if _, err = c.QuotePair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if f.batches != 33 {
		t.Fatal("did not refresh changed pool")
	}
	f.obj.Version++
	f.onRead = func() { c.ObserveCheckpoint(125) }
	if _, err = c.QuotePair(context.Background(), p); err == nil {
		t.Fatal("accepted concurrent newer notification")
	}
}

// TestStateCacheRejectsIncompleteTicks verifies incomplete snapshots never enter the cache.
//
// Version:
//   - 2026-09-08: Added.
func TestStateCacheRejectsIncompleteTicks(t *testing.T) {
	c, f, p := cacheFixture(t)
	f.partial = true
	if _, err := c.QuotePair(context.Background(), p); err == nil || c.snapshot != nil {
		t.Fatal("accepted incomplete tick set")
	}
}

type neighborFake struct {
	*stateReaderFake
	broken bool
	reads  int
}

// DynamicUint64ValuesAtCheckpoint provides the configured test behavior.
//
// Version:
//   - 2026-09-11: Delegate window batches to the keyed fixture.
func (f *neighborFake) DynamicUint64ValuesAtCheckpoint(_ context.Context, _ sui.Address, cp sui.CheckpointSequenceNumber, keys []uint64) ([]json.RawMessage, error) {
	if len(keys) != 2 {
		return f.stateReaderFake.DynamicUint64ValuesAtCheckpoint(context.Background(), sui.Address{}, cp, keys)
	}
	f.reads++
	if cp != f.cp || len(keys) != 2 || keys[0] != 443536 || keys[1] != 443736 {
		return nil, fmt.Errorf("wrong neighbor request")
	}
	next := 443736
	if f.broken {
		next = 443636
	}
	return []json.RawMessage{json.RawMessage(fmt.Sprintf(`{"score":"443536","nexts":[{"is_none":false,"v":"%d"}],"value":{"index":{"bits":4294967196},"sqrt_price":"9223372036854775808","liquidity_net":{"bits":"1000000"}}}`, next)), json.RawMessage(`{"score":"443736","prev":{"is_none":false,"v":"443536"},"value":{"index":{"bits":100},"sqrt_price":"36893488147419103232","liquidity_net":{"bits":"340282366920938463463374607431767211456"}}}`)}, nil
}

// TestStateNeighbors verifies fast refresh and fallback after an intervening tick appears.
//
// Version:
//   - 2026-09-11: Verify bounded fallback after changed neighbors.
func TestStateNeighbors(t *testing.T) {
	c, f, p := cacheFixture(t)
	n := &neighborFake{stateReaderFake: f}
	c.reader = n
	if _, err := c.QuotePair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	f.obj.Version++
	if _, err := c.QuotePair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if f.batches != 16 || n.reads != 1 {
		t.Fatal("neighbor refresh did not avoid bounded window refresh")
	}
	n.broken = true
	f.obj.Version++
	if _, err := c.QuotePair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if f.batches != 32 {
		t.Fatal("missing bounded refresh after link change")
	}
}

// TestQuoteCheckpointProgressIsMonotonic verifies quote-owned progress without trade observations.
//
// Version:
//   - 2026-09-08: Added.
func TestQuoteCheckpointProgressIsMonotonic(t *testing.T) {
	c, f, p := cacheFixture(t)
	first, err := c.QuotePair(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	f.cp = first.Checkpoint - 1
	if _, err := c.QuotePair(context.Background(), p); err == nil {
		t.Fatal("accepted checkpoint regression")
	}
	f.cp = first.Checkpoint
	if _, err := c.QuotePair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	f.cp = first.Checkpoint + 1
	next, err := c.QuotePair(context.Background(), p)
	if err != nil || next.Checkpoint != first.Checkpoint+1 {
		t.Fatalf("next=%+v err=%v", next, err)
	}
}

// DynamicUint64ValuesAtCheckpoint serves sparse aligned tick keys at the requested checkpoint.
//
// Version:
//   - 2026-09-11: Added.
func (f *stateReaderFake) DynamicUint64ValuesAtCheckpoint(_ context.Context, _ sui.Address, cp sui.CheckpointSequenceNumber, keys []uint64) ([]json.RawMessage, error) {
	if cp != f.cp || len(keys) > 50 {
		return nil, fmt.Errorf("invalid keyed request")
	}
	f.batches++
	if f.onRead != nil {
		f.onRead()
	}
	if f.partial {
		return nil, fmt.Errorf("failed to read test tick batch: response=incomplete")
	}
	result := make([]json.RawMessage, len(keys))
	for i, key := range keys {
		switch key {
		case 443536:
			result[i] = json.RawMessage(`{"value":{"index":{"bits":4294967196},"sqrt_price":"9223372036854775808","liquidity_net":{"bits":"1000000"}}}`)
		case 443736:
			result[i] = json.RawMessage(`{"value":{"index":{"bits":100},"sqrt_price":"36893488147419103232","liquidity_net":{"bits":"340282366920938463463374607431767211456"}}}`)
		}
	}
	return result, nil
}
