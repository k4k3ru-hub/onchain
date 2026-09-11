package dlmm

import (
	"context"
	"testing"
)

// TestWarmSeedsLocalQuoteWithoutTrial verifies state-only warming supports later RPC-free quotes.
//
// Version:
//   - 2026-09-12: Added.
func TestWarmSeedsLocalQuoteWithoutTrial(t *testing.T) {
	c, source, requests := newCacheFixture(t)
	if err := c.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := len(source.sizes)
	if _, err := c.QuoteRetainedExactInputs(context.Background(), requests); err != nil {
		t.Fatal(err)
	}
	if len(source.sizes) != calls {
		t.Fatal("retained quote fetched state")
	}
}
