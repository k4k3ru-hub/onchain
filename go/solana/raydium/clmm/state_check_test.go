package clmm

import (
	"context"
	"testing"
)

func TestCheckStateIncludesConfigAndTicks(t *testing.T) {
	c, a := cacheFixture(t)
	ctx := context.Background()
	if _, err := c.QuoteExactInputs(ctx, []ExactInputRequest{{InputMint: testAddress(4), AmountIn: 1000000}}); err != nil {
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
		t.Fatal("unchanged account inputs differ")
	}
	a.values[testAddress(2)].Data[47]++
	changed, err := c.CheckState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == changed.Key {
		t.Fatal("fee config change ignored")
	}
}
