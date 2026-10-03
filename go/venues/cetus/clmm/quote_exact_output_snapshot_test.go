package clmm

import (
	"context"
	"errors"
	"testing"
)

// TestExactOutputSnapshotIsDetached verifies fee-inclusive output targets after withdrawal.
//
// Version:
//   - 2026-10-02: Added.
func TestExactOutputSnapshotIsDetached(t *testing.T) {
	c, _, p := cacheFixture(t)
	ctx := t.Context()
	if err := c.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	frozen := c.CaptureQuoteSnapshot()
	before, err := frozen.QuotePair(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	c.retainedMu.Lock()
	c.retained = nil
	c.retainedMu.Unlock()
	got, err := frozen.QuoteExactOutput(ctx, p.Ask)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountOut != p.Ask.AmountOut || got.AmountIn != before.Ask.AmountIn || got.FeeAmount != before.Ask.FeeAmount {
		t.Fatal("detached output differs from retained pair", got, before.Ask)
	}
	// The frozen quote remains usable; neither input state nor caller params change.
	after, err := frozen.QuotePair(ctx, p)
	if err != nil || after.Ask.FeeAmount != before.Ask.FeeAmount {
		t.Fatal("snapshot mutated", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := frozen.QuoteExactOutput(canceled, p.Ask); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	bad := p.Ask
	bad.Pool.Address[0] ^= 1
	if _, err := frozen.QuoteExactOutput(ctx, bad); err == nil {
		t.Fatal("unrelated pool accepted")
	}
}
