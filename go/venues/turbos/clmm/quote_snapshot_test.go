package clmm

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

func TestFrozenQuoteSurvivesWithdrawal(t *testing.T) {
	c, _, p := stateFixture(t)
	ctx := context.Background()
	if _, err := c.QuotePair(ctx, p); err != nil {
		t.Fatal(err)
	}
	var current *QuoteSnapshot
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) { current = s })
	frozen := current
	if frozen == nil {
		t.Fatal("snapshot not published")
	}
	want, err := frozen.QuotePair(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	c.retainedMu.Lock()
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
			got.CapturedAt = want.CapturedAt
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("frozen calculation changed: %v", err)
			}
		}()
	}
	wg.Wait()
}
