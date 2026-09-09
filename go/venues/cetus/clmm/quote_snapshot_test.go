package clmm

import (
	"context"
	"testing"
)

func TestQuoteSnapshotSurvivesLiveWithdrawal(t *testing.T) {
	c, reader, p := cacheFixture(t)
	var current *QuoteSnapshot
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) { current = s })
	if current != nil {
		t.Fatal("uninitialized snapshot")
	}
	if err := c.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	frozen := current
	if frozen == nil {
		t.Fatal("warm did not publish")
	}
	expected, err := frozen.QuotePair(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	pages := reader.pages
	c.retainedMu.Lock()
	c.retained = nil
	c.publishQuoteSnapshotLocked()
	c.retainedMu.Unlock()
	if current != nil {
		t.Fatal("withdrawal not published")
	}
	reader.obj = nil
	actual, err := frozen.QuotePair(context.Background(), p)
	if err != nil || actual.Bid.AmountOut != expected.Bid.AmountOut || reader.pages != pages {
		t.Fatal("snapshot depends on live cache", err)
	}
}
