package clmm

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

// TestGrossPairUsesFrozenInputs verifies fee removal, isolated arithmetic and cancellation.
//
// Version:
//   - 2026-09-26: Added.
func TestGrossPairUsesFrozenInputs(t *testing.T) {
	c, _, p := stateFixture(t)
	ctx := context.Background()
	if _, err := c.QuotePair(ctx, p); err != nil {
		t.Fatal(err)
	}
	original := c.CaptureQuoteSnapshot()
	if original == nil {
		t.Fatal("missing snapshot")
	}
	before, err := original.QuotePair(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	c.retainedMu.Lock()
	c.retained = nil
	c.retainedMu.Unlock()
	frozen := original.cache.CaptureQuoteSnapshot()
	frozen.cache.retained.pool.Fee = 0
	pair, err := frozen.QuotePair(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := original.QuoteGrossPair(ctx, p)
			if err != nil {
				t.Error(err)
				return
			}
			if got.BidAmountOut.Cmp(pair.Bid.AmountOut) != 0 || got.AskAmountIn.Cmp(pair.Ask.AmountIn) != 0 {
				t.Error("gross differs from zero-fee simulation")
			}
			if got.BidFeeAmount.Uint64() != before.Bid.FeeAmount || got.AskFeeAmount.Uint64() != before.Ask.FeeAmount {
				t.Error("fee partition incorrect")
			}
			if got.BidFeeAmount.Sign() <= 0 || got.AskFeeAmount.Sign() <= 0 {
				t.Error("missing swap fees")
			}
		}()
	}
	wg.Wait()
	after, err := original.QuotePair(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	after.CapturedAt = before.CapturedAt
	if !reflect.DeepEqual(before, after) {
		t.Fatal("gross mutated normal quote")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := original.QuoteGrossPair(canceled, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
