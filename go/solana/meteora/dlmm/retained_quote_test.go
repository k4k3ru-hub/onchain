package dlmm

import (
	"context"
	"testing"
)

// TestRetainedQuoteMatchesBootstrapWithoutRPC verifies retained snapshot behavior.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedQuoteMatchesBootstrapWithoutRPC(t *testing.T) {
	cache, source, requests := newCacheFixture(t)
	first, err := cache.QuoteExactInputs(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	calls := len(source.sizes)
	cache.refresh.Lock()
	defer cache.refresh.Unlock()
	second, err := cache.QuoteRetainedExactInputs(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	if first.Quotes[0] != second.Quotes[0] || !first.ObservedAt.Equal(second.ObservedAt) {
		t.Fatal("retained quote differs from bootstrap")
	}
	if len(source.sizes) != calls {
		t.Fatal("retained calculation used RPC")
	}
}
