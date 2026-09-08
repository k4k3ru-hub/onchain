package cpmm

import (
	"context"
	"testing"
)

func TestCheckStateDetectsAccountChangeWithoutNotification(t *testing.T) {
	c, a := cacheFixture(t)
	ctx := context.Background()
	first, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key || first.Key == "" {
		t.Fatal("unchanged inputs not reusable")
	}
	a.values[testAddress(4)].Data[tokenAmountOffset]++
	changed, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Key == first.Key {
		t.Fatal("reserve change missed")
	}
	c.mu.Lock()
	c.minimumSlot++
	c.mu.Unlock()
	if _, err = c.CheckState(ctx); err == nil {
		t.Fatal("RPC behind observed slot accepted")
	}
}
