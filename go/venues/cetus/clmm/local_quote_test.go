package clmm

import (
	"math/big"
	"testing"
)

// TestLocalQuoteDirections checks integer rounding against manually derived Q64 cases.
//
// Version:
//   - 2026-09-08: Added.
func TestLocalQuoteDirections(t *testing.T) {
	q := new(big.Int).Lsh(big.NewInt(1), 64)
	s := LocalSnapshot{Pool: Pool{CurrentSqrtPrice: q, Liquidity: big.NewInt(1000000), FeeRate: 500, CurrentTickIndex: 0}, Ticks: []Tick{{Index: -100, SqrtPrice: new(big.Int).Div(new(big.Int).Set(q), big.NewInt(2)), LiquidityNet: big.NewInt(1000000)}, {Index: 100, SqrtPrice: new(big.Int).Mul(q, big.NewInt(2)), LiquidityNet: big.NewInt(-1000000)}}}
	for _, a2b := range []bool{false, true} {
		r, err := s.Quote(1000, a2b, true)
		if err != nil {
			t.Fatal(err)
		}
		if r.AmountIn != 999 || r.AmountOut != 998 || r.FeeAmount != 1 {
			t.Fatalf("input direction %v: %+v", a2b, r)
		}
		r, err = s.Quote(998, a2b, false)
		if err != nil {
			t.Fatal(err)
		}
		if r.AmountIn != 999 || r.AmountOut != 998 || r.FeeAmount != 1 {
			t.Fatalf("output direction %v: %+v", a2b, r)
		}
	}
	if s.Pool.CurrentSqrtPrice.Cmp(q) != 0 || s.Pool.Liquidity.Int64() != 1000000 {
		t.Fatal("mutated input")
	}
	if _, err := s.Quote(10000000, true, true); err == nil {
		t.Fatal("accepted incomplete fill")
	}
	s.Ticks = append(s.Ticks, s.Ticks[0])
	if _, err := s.Quote(1000, true, true); err == nil {
		t.Fatal("accepted duplicate tick")
	}
}

// TestLocalQuoteCrossing checks a zero-distance initialized tick and liquidity transition.
//
// Version:
//   - 2026-09-08: Added.
func TestLocalQuoteCrossing(t *testing.T) {
	q := new(big.Int).Lsh(big.NewInt(1), 64)
	s := LocalSnapshot{Pool: Pool{CurrentSqrtPrice: q, Liquidity: big.NewInt(2000000), CurrentTickIndex: 0}, Ticks: []Tick{{Index: 0, SqrtPrice: new(big.Int).Set(q), LiquidityNet: big.NewInt(1000000)}, {Index: -100, SqrtPrice: new(big.Int).Rsh(new(big.Int).Set(q), 1), LiquidityNet: big.NewInt(1000000)}}}
	r, err := s.Quote(1000, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if r.AmountOut != 999 {
		t.Fatalf("wrong crossed liquidity: %+v", r)
	}
}
