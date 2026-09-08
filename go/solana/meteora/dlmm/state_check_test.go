package dlmm

import (
	"context"
	"testing"
)

func TestCheckStateIncludesChainClock(t *testing.T) {
	c, a, req := newCacheFixture(t)
	ctx := context.Background()
	if _, err := c.QuoteExactInputs(ctx, req); err != nil {
		t.Fatal(err)
	}
	first, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key {
		t.Fatal("unchanged inputs differ")
	}
	a.values[clockSysvarAddress].Data[32]++
	changed, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == changed.Key {
		t.Fatal("time-dependent fee input ignored")
	}
}
