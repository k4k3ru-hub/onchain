package v4

import (
	"context"
	"math/big"
	"reflect"
	"sync"
	"testing"
)

func TestFrozenQuoteSurvivesWithdrawal(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()
	if _, err := c.QuotePair(ctx, big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	var current *QuoteSnapshot
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) { current = s })
	frozen := current
	if frozen == nil {
		t.Fatal("snapshot not published")
	}
	want, err := frozen.QuotePair(ctx, big.NewInt(1000000), true)
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.retained = nil
	c.publishQuoteSnapshotLocked()
	c.mu.Unlock()
	if current != nil {
		t.Fatal("withdrawal not published")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := frozen.QuotePair(ctx, big.NewInt(1000000), true)
			got.CapturedAt = want.CapturedAt
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("frozen calculation changed: %v", err)
			}
		}()
	}
	wg.Wait()
}
