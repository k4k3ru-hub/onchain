package clmm

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

// TestFrozenQuoteSurvivesWithdrawal verifies input receipt handling.
//
// Version:
//   - 2026-09-10: Preserve input receipt time.
func TestFrozenQuoteSurvivesWithdrawal(t *testing.T) {
	c, _, p := stateFixture(t)
	ctx := context.Background()
	if _, err := c.QuotePair(ctx, p); err != nil {
		t.Fatal(err)
	}
	received := time.Unix(1700000000, 123000).UTC()
	c.retained.received = received
	var current *QuoteSnapshot
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) { current = s })
	frozen := current
	if !frozen.ReceivedAt().Equal(received) {
		t.Fatal("receipt not captured")
	}
	if frozen == nil {
		t.Fatal("snapshot not published")
	}
	want, err := frozen.QuotePair(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	c.retainedMu.Lock()
	c.retained.received = received.Add(time.Hour)
	c.retained = nil
	c.publishQuoteSnapshotLocked()
	c.retainedMu.Unlock()
	if current != nil {
		t.Fatal("withdrawal not published")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := frozen.QuotePair(ctx, p)
			if !got.ReceivedAt.Equal(received) {
				t.Error("recalculation refreshed receipt")
			}
			got.CapturedAt = want.CapturedAt
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("frozen calculation changed: %v", err)
			}
		}()
	}
	wg.Wait()
}
