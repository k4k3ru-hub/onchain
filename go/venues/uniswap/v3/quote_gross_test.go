package v3

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"sync"
	"testing"
)

// TestGrossPairUsesFrozenInputs verifies fee removal, isolated arithmetic and cancellation.
//
// Version:
//   - 2026-09-26: Added.
func TestGrossPairUsesFrozenInputs(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()
	amount := big.NewInt(1000000)
	if _, err := c.QuotePair(ctx, amount, true); err != nil {
		t.Fatal(err)
	}
	original := c.CaptureQuoteSnapshot()
	if original == nil {
		t.Fatal("missing snapshot")
	}
	before, err := original.QuotePair(ctx, amount, true)
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.retained = nil
	c.mu.Unlock()
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := original.QuoteGrossPair(ctx, amount, true)
			if err != nil {
				t.Error(err)
				return
			}
			if got.BidAmountOut.Cmp(before.BidAmountOut) < 0 || got.AskAmountIn.Cmp(before.AskAmountIn) > 0 {
				t.Error("gross worsened price")
			}
			if new(big.Int).Sub(before.AskAmountIn, got.AskAmountIn).Cmp(got.AskFeeAmount) != 0 {
				t.Error("exact-output fee incorrect")
			}
			if got.BidFeeAmount.Sign() <= 0 || got.AskFeeAmount.Sign() <= 0 {
				t.Error("missing fees")
			}
		}()
	}
	wg.Wait()
	after, err := original.QuotePair(ctx, amount, true)
	if err != nil {
		t.Fatal(err)
	}
	after.CapturedAt = before.CapturedAt
	if !reflect.DeepEqual(before, after) {
		t.Fatal("gross mutated normal quote")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := original.QuoteGrossPair(canceled, amount, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	missing := original.cache.CaptureQuoteSnapshot()
	missing.cache.retained.words = nil
	if _, err := missing.QuoteGrossPair(ctx, amount, true); err == nil {
		t.Fatal("missing coverage accepted")
	}
}
