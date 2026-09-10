package clmm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"math/big"
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
//   - 2026-09-10: Supply typed dynamic-field fixtures.
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
	tick := &sui.Object{Move: &sui.MoveObject{Type: "0x2::dynamic_field::Field<" + c.retained.keyType + ",u256>", JSON: json.RawMessage(`{"name":{"bits":"10"},"value":{"liquidity_net":{"bits":"340282366920938463463374607431768211451"}}}`)}}
	word := &sui.Object{Move: &sui.MoveObject{Type: "0x2::dynamic_field::Field<" + c.retained.keyType + ",u256>", JSON: json.RawMessage(`{"name":{"bits":"0"},"value":"1024"}`)}}
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp, Successful: true}, ObjectChanges: []sui.ObjectChange{
		{Address: c.pool, Before: &before, After: &after},
		{OutputParent: c.retained.ticks, After: tick},
		{OutputParent: c.retained.bitmap, After: word},
	}}
	received := c.retained.received.Add(time.Second)
	if err := c.applyRetainedObjects(n, received); err != nil {
		t.Fatal(err)
	}
	if c.retained.nets[10].Int64() != -5 || c.retained.words[0].Uint64() != 1024 || c.retained.version != after.Version {
		t.Fatal("stream updates lost")
	}
	if !c.retained.received.Equal(received) {
		t.Fatal("accepted receipt lost")
	}
	if err := c.applyRetainedObjects(n, received.Add(time.Hour)); err != nil {
		t.Fatal("duplicate", err)
	}
	if !c.retained.received.Equal(received) {
		t.Fatal("duplicate refreshed receipt")
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

// TestRetainedMissingKeyDoesNotMutateBaseline verifies malformed fields cannot partially change held inputs.
//
// Version:
//   - 2026-09-10: Verify malformed indexed fields after type filtering.
//   - 2026-09-09: Added.
func TestRetainedMissingKeyDoesNotMutateBaseline(t *testing.T) {
	c, f, p := stateFixture(t)
	if _, err := c.QuotePair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	old := c.retained
	before := *f.obj
	after := before
	after.Version++
	cp := sui.CheckpointSequenceNumber(124)
	word := &sui.Object{Move: &sui.MoveObject{Type: "0x2::dynamic_field::Field<" + c.retained.keyType + ",u256>", JSON: json.RawMessage(`{"name":{"bits":"0"},"value":"1024"}`)}}
	invalid := &sui.Object{Move: &sui.MoveObject{Type: "0x2::dynamic_field::Field<" + c.retained.keyType + ",u256>", JSON: json.RawMessage(`{"name":{},"value":"0"}`)}}
	original := new(big.Int).Set(old.words[0])
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp, Successful: true}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, Before: &before, After: &after}, {OutputParent: old.bitmap, After: word}, {OutputParent: old.bitmap, After: invalid}}}
	if err := c.applyRetainedObjects(n); err == nil {
		t.Fatal("missing key accepted")
	}
	if old.words[0].Cmp(original) != 0 {
		t.Fatal("failed update mutated baseline")
	}
	if c.retained != nil {
		t.Fatal("incomplete state remained available")
	}
}

// TestRetainedUnrelatedFields verifies non-index fields preserve live pool updates and quotes.
//
// Version:
//   - 2026-09-10: Added.
func TestRetainedUnrelatedFields(t *testing.T) {
	for _, typ := range []string{
		"0x2::dynamic_field::Field<0x2::dynamic_object_field::Wrapper<0x1::string::String>,0x2::object::ID>",
		"0x0000000000000000000000000000000000000000000000000000000000000002::dynamic_field::Field<0x0000000000000000000000000000000000000000000000000000000000000002::dynamic_object_field::Wrapper<0x0000000000000000000000000000000000000000000000000000000000000001::string::String>,0x0000000000000000000000000000000000000000000000000000000000000002::object::ID>",
		"0x2::dynamic_field::Field<vector<u8>,bool>",
		"0x2::dynamic_field::Field<0x99::i32::I32,u256>",
	} {
		for _, remove := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deleted=%t", typ, remove), func(t *testing.T) {
				c, f, p := stateFixture(t)
				want, err := c.QuotePair(context.Background(), p)
				if err != nil {
					t.Fatal(err)
				}
				reads := f.reads
				before := *f.obj
				after := before
				after.Version++
				cp := sui.CheckpointSequenceNumber(124)
				obj := &sui.Object{Move: &sui.MoveObject{Type: typ, JSON: json.RawMessage(`{"name":{"name":"metadata"},"value":"0x3"}`)}}
				change := sui.ObjectChange{InputParent: c.pool, OutputParent: c.pool, Before: obj, After: obj}
				if remove {
					change.Deleted = true
					change.After = nil
					change.OutputParent = sui.Address{}
				}
				n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp, Successful: true}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, Before: &before, After: &after}, change}}
				if err := c.applyRetainedObjects(n); err != nil {
					t.Fatal(err)
				}
				// An unrelated child-only notification also needs no pool replacement.
				n.ObjectChanges = n.ObjectChanges[1:]
				if err := c.applyRetainedObjects(n); err != nil {
					t.Fatal(err)
				}
				if c.retained == nil || c.retained.version != after.Version {
					t.Fatal("pool update lost")
				}
				got, err := c.QuoteRetainedPair(context.Background(), p)
				if err != nil {
					t.Fatal(err)
				}
				if got.Bid.AmountOut.Cmp(want.Bid.AmountOut) != 0 || got.Ask.AmountIn.Cmp(want.Ask.AmountIn) != 0 || f.reads != reads {
					t.Fatal("quote changed or RPC invoked")
				}
			})
		}
	}
}
