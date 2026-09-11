package clmm

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

// TestFrozenAccountsSurviveDisconnect verifies immutable inputs and receipt time.
//
// Version:
//   - 2026-09-12: Verify receipt-order updates and retained inputs.
//   - 2026-09-10: Verify input receipt survives recalculation.
func TestFrozenAccountsSurviveDisconnect(t *testing.T) {
	c, _ := cacheFixture(t)
	requests := []ExactInputRequest{{InputMint: testAddress(4), AmountIn: 1000000}}
	ctx := context.Background()
	if _, err := c.QuoteExactInputs(ctx, requests); err != nil {
		t.Fatal(err)
	}
	c.connected = len(c.addresses)
	var current *QuoteSnapshot
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) { current = s })
	frozen := current
	if frozen == nil {
		t.Fatal("no frozen accounts")
	}
	want, err := frozen.QuoteExactInputs(ctx, requests)
	if err != nil {
		t.Fatal(err)
	}
	if want.ReceivedAt.IsZero() {
		t.Fatal("missing frozen input receipt")
	}
	c.mu.Lock()
	c.connected--
	c.invalidate(0)
	c.mu.Unlock()
	if current == nil {
		t.Fatal("partial disconnect discarded state")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := frozen.QuoteExactInputs(ctx, requests)
			got.CapturedAt = want.CapturedAt
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("frozen calculation changed: %v", err)
			}
		}()
	}
	wg.Wait()
}
