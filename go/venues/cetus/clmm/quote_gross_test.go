package clmm

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
	c, _, p := cacheFixture(t)
	ctx := context.Background()
	if err := c.Warm(ctx); err != nil {
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
	frozen.cache.retained.snapshot.Pool.FeeRate = 0
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
			if got.BidAmountOut.Cmp(new(big.Int).SetUint64(pair.Bid.AmountOut)) != 0 || got.AskAmountIn.Cmp(new(big.Int).SetUint64(pair.Ask.AmountIn)) != 0 {
				t.Error("gross differs from zero-fee simulation")
			}
			if got.BidFeeAmount.Uint64() != before.Bid.FeeAmount || got.AskFeeAmount.Uint64() != before.Ask.FeeAmount {
				t.Error("fee partition incorrect")
			}
			input, inputErr := original.QuoteGrossExactInput(ctx, p.Bid)
			if inputErr != nil {
				t.Error(inputErr)
				return
			}
			if input.AmountOut.Cmp(got.BidAmountOut) != 0 || input.FeeAmount.Cmp(got.BidFeeAmount) != 0 {
				t.Error("exact input disagrees with independently verified gross bid")
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

	if _, err := original.QuoteGrossExactInput(canceled, p.Bid); !errors.Is(err, context.Canceled) {
		t.Fatalf("exact input cancellation lost: %v", err)
	}
}

// TestGrossExactInputReverse verifies the opposite input token on a detached snapshot.
//
// Version:
//   - 2026-09-26: Added.
func TestGrossExactInputReverse(t *testing.T) {
	c, _, p := cacheFixture(t)
	if err := c.Warm(t.Context()); err != nil {
		t.Fatal(err)
	}
	frozen := c.CaptureQuoteSnapshot()
	reverse := p
	reverse.Bid.A2B, reverse.Ask.A2B = p.Ask.A2B, p.Bid.A2B

	pair, err := frozen.QuoteGrossPair(t.Context(), reverse)
	if err != nil {
		t.Fatal(err)
	}
	got, err := frozen.QuoteGrossExactInput(t.Context(), reverse.Bid)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountOut.Cmp(pair.BidAmountOut) != 0 || got.FeeAmount.Cmp(pair.BidFeeAmount) != 0 {
		t.Fatal("reverse input gross amount or fee differs")
	}
}
