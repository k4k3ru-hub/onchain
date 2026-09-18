package clliquidity

import (
	"encoding/json"
	"math/big"
	"testing"
)

// TestApplyLiquidityAndSwap checks complete-range updates and terminal swaps.
//
// Version:
//   - 2026-09-19: Added.
func TestApplyLiquidityAndSwap(t *testing.T) {
	s := State{Complete: true, Spacing: 10, Tick: 0, SqrtPriceX96: sqrtAtTick(0), ActiveLiquidity: new(big.Int)}
	var err error
	for _, u := range []Update{
		{Lower: -10, Upper: 10, LiquidityDelta: big.NewInt(100)},
		{Lower: 10, Upper: 30, LiquidityDelta: big.NewInt(200)},
		{SqrtPriceX96: sqrtAtTick(20), Tick: 20, ActiveLiquidity: big.NewInt(200)},
		{Lower: -10, Upper: 10, LiquidityDelta: big.NewInt(-100)},
		{Lower: 10, Upper: 30, LiquidityDelta: big.NewInt(-30)},
	} {
		before, _ := json.Marshal(s)
		next, e := Apply(s, u, 4096)
		err = e
		if err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(s)
		if string(before) != string(after) {
			t.Fatal("mutated input")
		}
		s = next
	}
	if len(s.Ticks) != 2 || s.Ticks[0].Index != 10 || s.ActiveLiquidity.Cmp(big.NewInt(170)) != 0 {
		t.Fatalf("state %+v", s)
	}
	before, _ := json.Marshal(s)
	if _, err = Apply(s, Update{Lower: 10, Upper: 30, LiquidityDelta: big.NewInt(-171)}, 4096); err == nil {
		t.Fatal("negative liquidity accepted")
	}
	if _, err = Apply(s, Update{Lower: 40, Upper: 50, LiquidityDelta: big.NewInt(1)}, 2); err == nil {
		t.Fatal("tick budget ignored")
	}
	if _, err = Apply(s, Update{SqrtPriceX96: sqrtAtTick(0), Tick: 0, ActiveLiquidity: big.NewInt(1)}, 4096); err == nil {
		t.Fatal("inconsistent swap accepted")
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("failure mutated input")
	}
}
