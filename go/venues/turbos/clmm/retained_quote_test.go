package clmm

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"testing"
	"time"
)

// TestRetainedPairDoesNotReadOrWait verifies detached local quotes despite unavailable RPC and Trade progress.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedPairDoesNotReadOrWait(t *testing.T) {
	c, f, p := stateFixture(t)
	want, err := c.QuotePair(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	reads := f.reads
	f.fail = errors.New("RPC unavailable")
	c.ObserveCheckpoint(999)
	c.gate <- struct{}{}
	defer func() { <-c.gate }()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	capturedAfter := time.Now()
	got, err := c.QuoteRetainedPair(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.CapturedAt.Before(capturedAfter) || got.CapturedAt.After(time.Now()) || !got.StateTimestamp.Equal(want.StateTimestamp) {
		t.Fatal("capture time must be fresh without relabelling the baseline")
	}
	if got.Bid.AmountOut.Cmp(want.Bid.AmountOut) != 0 || got.Ask.AmountIn.Cmp(want.Ask.AmountIn) != 0 || f.reads != reads {
		t.Fatalf("quote changed or RPC used: got=%+v reads=%d/%d", got, f.reads, reads)
	}
	c.retainedMu.Lock()
	frozen := cloneRetainedState(c.retained)
	c.retained.pool.Liquidity.SetInt64(1)
	c.retainedMu.Unlock()
	if frozen.pool.Liquidity.Int64() == 1 {
		t.Fatal("snapshot aliases retained pool")
	}
	c.retainedMu.Lock()
	c.retained.words = nil
	c.retainedMu.Unlock()
	if _, err := c.QuoteRetainedPair(ctx, p); err == nil {
		t.Fatal("missing bitmap must fail without supplementation")
	}
	if f.reads != reads {
		t.Fatal("missing coverage invoked RPC")
	}
}

// TestRetainedObjectUpdates verifies pool, bitmap and signed tick updates, ownership removal and full replacements across version gaps.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedObjectUpdates(t *testing.T) {
	c, f, p := stateFixture(t)
	if _, err := c.QuotePair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	before := *f.obj
	after := before
	after.Version++
	cp := sui.CheckpointSequenceNumber(124)
	tick := &sui.Object{Move: &sui.MoveObject{JSON: json.RawMessage(`{"name":{"bits":"10"},"value":{"liquidity_net":{"bits":"340282366920938463463374607431768211451"}}}`)}}
	word := &sui.Object{Move: &sui.MoveObject{JSON: json.RawMessage(`{"name":{"bits":"0"},"value":"1024"}`)}}
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp, Successful: true}, ObjectChanges: []sui.ObjectChange{
		{Address: c.pool, Before: &before, After: &after},
		{OutputParent: c.retained.ticks, After: tick},
		{OutputParent: c.retained.bitmap, After: word},
	}}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal(err)
	}
	if c.retained.nets[10].Int64() != -5 || c.retained.words[0].Uint64() != 1024 || c.retained.version != after.Version {
		t.Fatal("stream updates lost")
	}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal("duplicate", err)
	}
	before = after
	after.Version++
	n.ObjectChanges = n.ObjectChanges[:2]
	n.ObjectChanges[1] = sui.ObjectChange{InputParent: c.retained.ticks, Before: tick, After: tick}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.retained.nets[10]; ok {
		t.Fatal("moved tick retained")
	}
	after.Version++
	if err := c.applyRetainedObjects(n); err != nil || c.retained == nil {
		t.Fatal("full replacement rejected across input version gap", err)
	}
	n.ObjectChanges = n.ObjectChanges[:1]
	n.ObjectChanges[0].Before = nil
	after.Version++
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal("full replacement required before payload", err)
	}
	n.ObjectChanges[0].After = &sui.Object{Address: c.pool, Version: after.Version + 1}
	if err := c.applyRetainedObjects(n); err == nil || c.retained != nil {
		t.Fatal("malformed pool was accepted")
	}
}

// TestRetainedVersionGapStillQuotesLocally verifies full pool replacements without network or version waits.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedVersionGapStillQuotesLocally(t *testing.T) {
	c, f, p := stateFixture(t)
	if _, err := c.QuotePair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	reads := f.reads
	f.fail = errors.New("RPC unavailable")
	c.gate <- struct{}{}
	defer func() { <-c.gate }()
	before := *f.obj
	before.Version += 9
	after := before
	after.Version++
	cp := sui.CheckpointSequenceNumber(124)
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp, Successful: true}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, Before: &before, After: &after}}}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	got, err := c.QuoteRetainedPair(ctx, p)
	if err != nil || got.Bid.AmountOut.Sign() <= 0 || got.Ask.AmountIn.Sign() <= 0 {
		t.Fatal("quote failed after version gap", err)
	}
	if c.retained.version != after.Version || f.reads != reads {
		t.Fatal("replacement missing or RPC used")
	}
	older := *f.obj
	n.ObjectChanges[0].After = &older
	if err := c.applyRetainedObjects(n); err != nil || c.retained.version != after.Version {
		t.Fatal("older notification rolled back state", err)
	}
}
