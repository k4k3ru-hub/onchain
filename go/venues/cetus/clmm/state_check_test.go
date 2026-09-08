package clmm

import (
	"context"
	"testing"
)

func TestCheckStatePoolAndTicksWithoutSwap(t *testing.T) {
	c, f, p := cacheFixture(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := c.QuotePair(ctx, p); err != nil {
			t.Fatal(err)
		}
		if _, err := c.CheckState(ctx); err != nil {
			t.Fatal(err)
		}
	}
	a, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.cp++
	b, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Key != b.Key || b.Position <= a.Position {
		t.Fatal("head advance unnecessarily changed inputs")
	}
	f.obj.Version++
	changed, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Key == b.Key {
		t.Fatal("pool change ignored")
	}
	c.ObserveCheckpoint(f.cp + 1)
	if _, err := c.CheckState(ctx); err == nil {
		t.Fatal("unobserved state accepted")
	}
}
