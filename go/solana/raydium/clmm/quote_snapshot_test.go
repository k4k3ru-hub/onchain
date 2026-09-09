package clmm

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

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
	c.mu.Lock()
	c.connected--
	c.invalidate(0)
	c.mu.Unlock()
	if current != nil {
		t.Fatal("partial disconnect did not withdraw state")
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
