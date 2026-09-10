package clmm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

// TestRetainedQuoteUsesStreamWithoutCheckpointReads verifies frozen inputs and no RPC fallback.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedQuoteUsesStreamWithoutCheckpointReads(t *testing.T) {
	c, reader, params := cacheFixture(t)
	ctx := context.Background()
	if err := c.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := c.QuoteRetainedPair(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	pages := reader.pages
	reader.obj = nil     // Any attempt to supplement the quote now fails.
	c.gate <- struct{}{} // Local quotes must not wait for an in-flight initialization.
	defer func() { <-c.gate }()
	c.ObserveCheckpoint(999)
	frozen := cloneLocalSnapshot(c.retained.snapshot)
	_, fixture, _ := cacheFixture(t)
	after := *fixture.obj
	after.Version++
	move := *fixture.obj.Move
	move.JSON = json.RawMessage(strings.Replace(string(move.JSON), `"fee_rate":"500"`, `"fee_rate":"10000"`, 1))
	after.Move = &move
	cp := sui.CheckpointSequenceNumber(124)
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp, TransactionIndex: new(uint64), Successful: true}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, Before: fixture.obj, After: &after}}}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal(err)
	}
	inputs := c.CaptureQuoteSnapshot().Inputs()
	if inputs.Position.Kind != "transaction" || inputs.Position.Sequence != cp.Uint64() || inputs.Position.Index == nil || *inputs.Position.Index != 0 || inputs.Baseline.Sequence >= inputs.Position.Sequence {
		t.Fatal("lost retained transaction position", inputs)
	}

	second, err := c.QuoteRetainedPair(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if second.Bid.AmountOut >= first.Bid.AmountOut || second.Ask.AmountIn+second.Ask.FeeAmount <= first.Ask.AmountIn+first.Ask.FeeAmount {
		t.Fatal("streamed fee not used")
	}
	if reader.pages != pages || frozen.Pool.FeeRate != 500 || second.Checkpoint != first.Checkpoint {
		t.Fatal("RPC, mutation or relabelled baseline")
	}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal("duplicate rejected", err)
	}
	n.ObjectChanges[0].After = &sui.Object{Address: c.pool, Version: after.Version + 2, Move: after.Move}
	if err := c.applyRetainedObjects(n); err != nil || c.retained == nil {
		t.Fatal("full replacement rejected across input version gap", err)
	}
	if _, err := c.QuoteRetainedPair(ctx, params); err != nil {
		t.Fatal("quote failed after replacement", err)
	}
	n.ObjectChanges[0].Before = nil
	n.ObjectChanges[0].After.Version++
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal("full replacement required before payload", err)
	}
	n.ObjectChanges[0].After = &sui.Object{Address: c.pool, Version: c.retained.version + 1}
	if err := c.applyRetainedObjects(n); err == nil || c.retained != nil {
		t.Fatal("malformed pool was accepted")
	}
}

// TestRetainedTickObjectsUpdatesAndDeletes verifies full dynamic-field payloads.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedTickObjectsUpdatesAndDeletes(t *testing.T) {
	c, reader, params := cacheFixture(t)
	if err := c.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := *reader.obj
	after := before
	after.Version++
	id, err := sui.ParseAddress("0x77")
	if err != nil {
		t.Fatal(err)
	}
	tick := &sui.Object{Address: id, Version: 11, Move: &sui.MoveObject{JSON: json.RawMessage(`{"id":"0x77","name":"100","value":{"value":{"index":{"bits":100},"sqrt_price":"36893488147419103232","liquidity_net":{"bits":"340282366920938463463374607431767201456"}}}}`)}}
	cp := sui.CheckpointSequenceNumber(124)
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp, Successful: true}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, Before: &before, After: &after}, {Address: id, OutputParent: c.retained.handle, After: tick}}}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal(err)
	}
	if c.retained.snapshot.Ticks[1].LiquidityNet.Int64() != -1010000 {
		t.Fatal("tick update lost")
	}
	before = after
	after.Version++
	n.ObjectChanges[1] = sui.ObjectChange{Address: id, InputParent: c.retained.handle, Before: tick, Deleted: true}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal(err)
	}
	if len(c.retained.snapshot.Ticks) != 1 {
		t.Fatal("tick deletion lost")
	}
	if _, err := c.QuoteRetainedPair(context.Background(), params); err == nil {
		t.Fatal("missing directional tick coverage accepted")
	}
}
