// Package clliquidity evaluates full-range concentrated-liquidity principal.
// It excludes fees, direct transfers and removed but uncollected amounts.
package clliquidity

import (
	"fmt"
	"math/big"
)

type Tick struct {
	Index      int32
	Gross, Net *big.Int
}

type State struct {
	SqrtPriceX96    *big.Int
	Tick            int32
	Spacing         int32
	ActiveLiquidity *big.Int
	Ticks           []Tick
	// Complete is true only after every bitmap word and initialized tick was read.
	Complete bool
}

type Amounts struct{ Token0, Token1 *big.Int }

// Calculate derives current principal amounts from a complete tick distribution.
// Each interval is rounded down to token base units; these are estimates, not
// exact per-position withdrawal amounts. Inputs are never mutated.
//
// Version:
//   - 2026-09-19: Added.
func Calculate(s State) (Amounts, error) {
	fail := func(reason string) (Amounts, error) {
		return Amounts{}, fmt.Errorf("failed to calculate lp principal: %s", reason)
	}
	if !s.Complete {
		return fail("state=incomplete")
	}
	if s.Spacing < 1 || s.Spacing > 32767 || s.Tick < -887272 || s.Tick >= 887272 {
		return fail("coordinates=out_of_range")
	}
	if s.SqrtPriceX96 == nil || s.ActiveLiquidity == nil {
		return fail("state=null")
	}
	if s.SqrtPriceX96.Cmp(sqrtAtTick(-887272)) < 0 || s.SqrtPriceX96.Cmp(sqrtAtTick(887272)) >= 0 || s.ActiveLiquidity.Sign() < 0 || s.ActiveLiquidity.BitLen() > 128 {
		return fail("state=out_of_range")
	}
	// At an exact boundary after a zero-for-one swap, slot0.tick may be one
	// below the mathematical price tick. The inclusive upper check permits it.
	if s.SqrtPriceX96.Cmp(sqrtAtTick(s.Tick)) < 0 || s.SqrtPriceX96.Cmp(sqrtAtTick(s.Tick+1)) > 0 {
		return fail("price_tick=mismatch")
	}
	for i, t := range s.Ticks {
		if t.Index < -887272 || t.Index > 887272 || t.Index%s.Spacing != 0 || i > 0 && t.Index <= s.Ticks[i-1].Index {
			return fail("ticks=invalid")
		}
		if t.Gross == nil || t.Net == nil {
			return fail("tick_liquidity=null")
		}
		if t.Gross.Sign() <= 0 || t.Gross.BitLen() > 128 || t.Net.Cmp(new(big.Int).Neg(power2(127))) < 0 || t.Net.Cmp(power2(127)) >= 0 || new(big.Int).Abs(t.Net).Cmp(t.Gross) > 0 {
			return fail("tick_liquidity=invalid")
		}
	}
	result := Amounts{new(big.Int), new(big.Int)}
	l, active := new(big.Int), new(big.Int)
	for i, t := range s.Ticks {
		l.Add(l, t.Net)
		if l.Sign() < 0 || l.BitLen() > 128 {
			return fail("cumulative_liquidity=out_of_range")
		}
		if t.Index <= s.Tick {
			active.Set(l)
		}
		if i+1 == len(s.Ticks) {
			continue
		}
		a, b := sqrtAtTick(t.Index), sqrtAtTick(s.Ticks[i+1].Index)
		c := new(big.Int).Set(s.SqrtPriceX96)
		if c.Cmp(a) < 0 {
			c.Set(a)
		}
		if c.Cmp(b) > 0 {
			c.Set(b)
		}
		result.Token0.Add(result.Token0, amountDelta(c, b, l, true, false))
		result.Token1.Add(result.Token1, amountDelta(a, c, l, false, false))
	}
	if l.Sign() != 0 || active.Cmp(s.ActiveLiquidity) != 0 {
		return fail("liquidity=mismatch")
	}
	return result, nil
}

// Value converts principal base units using explicit USD-equivalent unit prices.
// Callers own token identity, reference-price provenance and freshness checks.
//
// Version:
//   - 2026-09-19: Added.
func Value(a Amounts, decimals0, decimals1 uint8, price0, price1 *big.Rat) (*big.Rat, error) {
	if a.Token0 == nil || a.Token1 == nil || price0 == nil || price1 == nil {
		return nil, fmt.Errorf("failed to value lp principal: input=null")
	}
	if a.Token0.Sign() < 0 || a.Token1.Sign() < 0 || price0.Sign() <= 0 || price1.Sign() <= 0 {
		return nil, fmt.Errorf("failed to value lp principal: input=out_of_range")
	}
	unit := func(amount *big.Int, dec uint8, price *big.Rat) *big.Rat {
		return new(big.Rat).Mul(new(big.Rat).SetFrac(amount, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dec)), nil)), price)
	}
	return new(big.Rat).Add(unit(a.Token0, decimals0, price0), unit(a.Token1, decimals1, price1)), nil
}

// PriceRatio returns token1 units per token0 unit from the pool square-root price.
// It is a pool spot price, not an independent oracle price.
//
// Version:
//   - 2026-09-19: Added.
func PriceRatio(price *big.Int, decimals0, decimals1 uint8) (*big.Rat, error) {
	if price == nil || price.Cmp(sqrtAtTick(-887272)) < 0 || price.Cmp(sqrtAtTick(887272)) >= 0 {
		return nil, fmt.Errorf("failed to calculate lp price: sqrt_price=out_of_range")
	}
	n := new(big.Int).Mul(price, price)
	n.Mul(n, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals0)), nil))
	d := new(big.Int).Mul(power2(192), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals1)), nil))
	return new(big.Rat).SetFrac(n, d), nil
}
