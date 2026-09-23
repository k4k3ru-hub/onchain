package clliquidity

import (
	"math/big"
	"testing"
)

// TestPositionPrincipal verifies unrounded principal and integer withdrawal bounds.
//
// Version:
//   - 2026-09-23: Added.
func TestPositionPrincipal(t *testing.T) {
	for _, tick := range []int32{-120, 0, 120} {
		price, liquidity := sqrtAtTick(tick), big.NewInt(100000000)
		a, b, err := PositionPrincipal(price, liquidity, -60, 60)
		if err != nil {
			t.Fatal(err)
		}
		c := new(big.Int).Set(price)
		lo, hi := sqrtAtTick(-60), sqrtAtTick(60)
		if c.Cmp(lo) < 0 {
			c.Set(lo)
		}
		if c.Cmp(hi) > 0 {
			c.Set(hi)
		}
		if new(big.Int).Quo(a.Num(), a.Denom()).Cmp(amountDelta(c, hi, liquidity, true, false)) != 0 || new(big.Int).Quo(b.Num(), b.Denom()).Cmp(amountDelta(lo, c, liquidity, false, false)) != 0 {
			t.Fatal("integer withdrawal mismatch")
		}
		if price.Cmp(sqrtAtTick(tick)) != 0 || liquidity.Int64() != 100000000 {
			t.Fatal("mutated input")
		}
	}
	a, b, err := PositionPrincipal(sqrtAtTick(0), big.NewInt(1), -60, 60)
	if err != nil || a.Sign() <= 0 || b.Sign() <= 0 || a.Cmp(big.NewRat(1, 1)) >= 0 {
		t.Fatal("sub-unit principal lost")
	}
	for _, lower := range []int32{-887273, 60} {
		if _, _, err := PositionPrincipal(sqrtAtTick(0), big.NewInt(1), lower, 60); err == nil {
			t.Fatal("invalid range accepted")
		}
	}
}
