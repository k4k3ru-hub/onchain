package clmm

import (
	"context"
	"testing"
)

// TestWarmSeedsLocalQuoteWithoutTrial verifies state-only warming supports later RPC-free quotes.
//
// Version:
//   - 2026-09-12: Added.
func TestWarmSeedsLocalQuoteWithoutTrial(t *testing.T) {
	c, source := cacheFixture(t)
	requests := []ExactInputRequest{{InputMint: testAddress(4), AmountIn: 1000}}
	if err := c.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := source.snapshotCalls
	if _, err := c.QuoteRetainedExactInputs(context.Background(), requests); err != nil {
		t.Fatal(err)
	}
	if source.snapshotCalls != calls {
		t.Fatal("retained quote fetched state")
	}
}
