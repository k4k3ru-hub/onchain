package clliquidity

import (
	"fmt"
	"math/big"
	"sort"
)

// Update describes a terminal Swap state or one signed LP liquidity change.
// Exactly one of SqrtPriceX96 and LiquidityDelta must be set.
type Update struct {
	SqrtPriceX96    *big.Int
	Tick            int32
	ActiveLiquidity *big.Int
	Lower, Upper    int32
	LiquidityDelta  *big.Int
}

// Clone copies complete state without sharing mutable integers or tick storage.
//
// Version:
//   - 2026-09-19: Added.
func Clone(s State) State {
	integer := func(n *big.Int) *big.Int {
		if n == nil {
			return nil
		}
		return new(big.Int).Set(n)
	}
	s.SqrtPriceX96 = integer(s.SqrtPriceX96)
	s.ActiveLiquidity = integer(s.ActiveLiquidity)
	ticks := make([]Tick, len(s.Ticks))
	for i, t := range s.Ticks {
		ticks[i] = Tick{Index: t.Index, Gross: integer(t.Gross), Net: integer(t.Net)}
	}
	s.Ticks = ticks
	return s
}

// Apply applies one ordered update to an owned copy and verifies full-state consistency.
// Log ordering, deduplication and canonicality belong to the stream owner.
// Failed updates never mutate input state. maxTicks bounds growth from new ranges.
//
// Version:
//   - 2026-09-19: Added.
func Apply(s State, u Update, maxTicks int) (State, error) {
	fail := func(reason string) (State, error) {
		return State{}, fmt.Errorf("failed to apply lp update: %s", reason)
	}
	if maxTicks < 1 || len(s.Ticks) > maxTicks {
		return fail("ticks=too_long")
	}
	if _, err := calculate(s, false); err != nil {
		return State{}, fmt.Errorf("failed to apply lp update: %w", err)
	}
	if (u.SqrtPriceX96 == nil) == (u.LiquidityDelta == nil) {
		return fail("update=invalid")
	}
	next := Clone(s)
	if u.SqrtPriceX96 != nil {
		if u.ActiveLiquidity == nil {
			return fail("active_liquidity=null")
		}
		next.SqrtPriceX96 = new(big.Int).Set(u.SqrtPriceX96)
		next.Tick = u.Tick
		next.ActiveLiquidity = new(big.Int).Set(u.ActiveLiquidity)
	} else {
		if u.Lower >= u.Upper || u.Lower < -887272 || u.Upper > 887272 || u.Lower%s.Spacing != 0 || u.Upper%s.Spacing != 0 || u.LiquidityDelta.BitLen() > 128 {
			return fail("range=invalid")
		}
		for _, index := range []int32{u.Lower, u.Upper} {
			at := sort.Search(len(next.Ticks), func(i int) bool { return next.Ticks[i].Index >= index })
			if at == len(next.Ticks) || next.Ticks[at].Index != index {
				if u.LiquidityDelta.Sign() < 0 {
					return fail("tick=missing")
				}
				if u.LiquidityDelta.Sign() == 0 {
					continue
				}
				next.Ticks = append(next.Ticks, Tick{})
				copy(next.Ticks[at+1:], next.Ticks[at:])
				next.Ticks[at] = Tick{Index: index, Gross: new(big.Int), Net: new(big.Int)}
			}
			t := &next.Ticks[at]
			t.Gross.Add(t.Gross, u.LiquidityDelta)
			if index == u.Lower {
				t.Net.Add(t.Net, u.LiquidityDelta)
			} else {
				t.Net.Sub(t.Net, u.LiquidityDelta)
			}
			if t.Gross.Sign() == 0 {
				if t.Net.Sign() != 0 {
					return fail("tick_liquidity=mismatch")
				}
				next.Ticks = append(next.Ticks[:at], next.Ticks[at+1:]...)
			}
		}
		if next.Tick >= u.Lower && next.Tick < u.Upper {
			next.ActiveLiquidity.Add(next.ActiveLiquidity, u.LiquidityDelta)
		}
	}
	if len(next.Ticks) > maxTicks {
		return fail("ticks=too_long")
	}
	if _, err := calculate(next, false); err != nil {
		return State{}, fmt.Errorf("failed to apply lp update: %w", err)
	}
	return next, nil
}
