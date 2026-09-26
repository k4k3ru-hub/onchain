package v3

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"sync"
	"testing"
)

// TestGrossPairUsesFrozenInputs verifies gross pair and exact-input isolation and cancellation.
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
			input, inputErr := original.QuoteGrossExactInput(ctx, amount, true)
			if inputErr != nil {
				t.Error(inputErr)
				return
			}
			if input.AmountOut.Cmp(got.BidAmountOut) != 0 || input.FeeAmount.Cmp(got.BidFeeAmount) != 0 {
				t.Error("exact input disagrees with independently verified gross bid")
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
	if _, err := original.QuoteGrossExactInput(canceled, amount, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("exact input cancellation lost: %v", err)
	}
	missing := original.cache.CaptureQuoteSnapshot()
	missing.cache.retained.words = nil
	if _, err := missing.QuoteGrossPair(ctx, amount, true); err == nil {
		t.Fatal("missing coverage accepted")
	}
}

// TestGrossExactInputOppositeDirection verifies reverse inputs and independent result ownership.
//
// Version:
//   - 2026-09-26: Added.
func TestGrossExactInputOppositeDirection(t *testing.T) {
	c, _ := newTestCache(t)
	if _, err := c.QuotePair(t.Context(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	snapshot := c.CaptureQuoteSnapshot()
	amount := big.NewInt(1000000)
	before, err := snapshot.QuotePair(t.Context(), amount, false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := snapshot.QuoteGrossExactInput(t.Context(), amount, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.AmountOut.Cmp(before.BidAmountOut) < 0 || first.FeeAmount.Sign() <= 0 || amount.Int64() != 1000000 {
		t.Fatal("reverse input or fee incorrect")
	}
	first.AmountOut.SetInt64(0)
	second, err := snapshot.QuoteGrossExactInput(t.Context(), amount, false)
	if err != nil || second.AmountOut.Sign() <= 0 {
		t.Fatal("returned amounts alias frozen state", err)
	}
}
