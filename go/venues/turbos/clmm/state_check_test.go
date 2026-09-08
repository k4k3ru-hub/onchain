package clmm

import (
	"context"
	"testing"
)

func TestCheckStateVerifiesChildrenAndCheckpoint(t *testing.T) {
	c, f, p := stateFixture(t)
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
	b, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Key != b.Key || a.Key == "" {
		t.Fatal("unchanged inputs not reusable")
	}
	f.obj.Version++
	changed, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Key == changed.Key {
		t.Fatal("object update ignored")
	}
	c.ObserveCheckpoint(f.head.SequenceNumber + 1)
	if _, err = c.CheckState(ctx); err == nil {
		t.Fatal("head behind notification accepted")
	}
}
