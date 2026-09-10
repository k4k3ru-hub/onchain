package clmm

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/k4k3ru-hub/onchain/go/internal/suiwindow"
)

type Tick struct {
	Index                   int32
	SqrtPrice, LiquidityNet *big.Int
}
type LocalSnapshot struct {
	Pool   Pool
	Ticks  []Tick
	window *tickWindow
}

// Quote computes a complete quote including an explicit fee amount from a caller-owned coherent snapshot.
// AmountIn excludes FeeAmount, matching Cetus simulation return values.
// The snapshot must contain all ticks traversed by the quote. Inputs are never mutated.
//
// Version:
//   - 2026-09-11: Stop bounded quotes at verified coverage edges, including empty intervals.
//   - 2026-09-08: Added.
func (s LocalSnapshot) Quote(amount uint64, a2b, exactInput bool) (QuoteResult, error) {
	fail := func(reason string) (QuoteResult, error) {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus local state: %s", reason)
	}
	p := s.Pool
	if amount == 0 || p.Paused || p.FeeRate >= 1000000 || p.CurrentSqrtPrice == nil || p.Liquidity == nil || p.CurrentSqrtPrice.Sign() <= 0 || p.Liquidity.Sign() < 0 || p.Liquidity.BitLen() > 128 {
		return fail("state=invalid")
	}
	ticks := append([]Tick(nil), s.Ticks...)
	if w := s.window; w != nil {
		if p.CurrentTickIndex < w.lower || p.CurrentTickIndex >= w.upper || p.CurrentSqrtPrice.Cmp(sqrtAtTick(w.lower)) < 0 || p.CurrentSqrtPrice.Cmp(sqrtAtTick(w.upper)) > 0 {
			return QuoteResult{}, fmt.Errorf("failed to quote cetus local state: %w", suiwindow.ErrCoverage)
		}
		for _, boundary := range []int32{w.lower, w.upper} {
			found := false
			for _, t := range ticks {
				if t.Index == boundary {
					found = true
					break
				}
			}
			if !found {
				ticks = append(ticks, Tick{Index: boundary, SqrtPrice: sqrtAtTick(boundary), LiquidityNet: new(big.Int)})
			}
		}
	}
	sort.Slice(ticks, func(i, j int) bool { return ticks[i].Index < ticks[j].Index })
	for i, t := range ticks {
		if t.Index < -443636 || t.Index > 443636 || t.SqrtPrice == nil || t.SqrtPrice.Sign() <= 0 || t.LiquidityNet == nil || t.LiquidityNet.BitLen() > 128 {
			return fail("tick=invalid")
		}
		if i > 0 && (ticks[i-1].Index == t.Index || ticks[i-1].SqrtPrice.Cmp(t.SqrtPrice) >= 0) {
			return fail("tick_order=invalid")
		}
	}
	price := new(big.Int).Set(p.CurrentSqrtPrice)
	liq := new(big.Int).Set(p.Liquidity)
	remaining := new(big.Int).SetUint64(amount)
	totalIn, totalOut, totalFee := new(big.Int), new(big.Int), new(big.Int)
	if a2b {
		for i, j := 0, len(ticks)-1; i < j; i, j = i+1, j-1 {
			ticks[i], ticks[j] = ticks[j], ticks[i]
		}
	}
	for _, t := range ticks {
		if (a2b && t.Index > p.CurrentTickIndex) || (!a2b && t.Index <= p.CurrentTickIndex) {
			continue
		}
		target := t.SqrtPrice
		if (a2b && target.Cmp(price) > 0) || (!a2b && target.Cmp(price) < 0) {
			return fail("tick_price=invalid")
		}
		if liq.Sign() == 0 {
			price = new(big.Int).Set(target)
		} else {
			in, out, next, fee, err := localStep(price, target, liq, remaining, p.FeeRate, a2b, exactInput)
			if err != nil {
				return QuoteResult{}, fmt.Errorf("failed to quote cetus local state: %w", err)
			}
			used := out
			if exactInput {
				used = new(big.Int).Add(in, fee)
			}
			if used.Cmp(remaining) > 0 {
				return fail("consumption=out_of_range")
			}
			remaining.Sub(remaining, used)
			totalIn.Add(totalIn, in)
			totalIn.Add(totalIn, fee)
			totalOut.Add(totalOut, out)
			totalFee.Add(totalFee, fee)
			price = next
		}
		if remaining.Sign() == 0 {
			break
		}
		if price.Cmp(target) == 0 {
			if a2b {
				liq.Sub(liq, t.LiquidityNet)
			} else {
				liq.Add(liq, t.LiquidityNet)
			}
			if liq.Sign() < 0 || liq.BitLen() > 128 {
				return fail("liquidity=out_of_range")
			}
		}
	}
	if remaining.Sign() != 0 {
		if s.window != nil {
			return QuoteResult{}, fmt.Errorf("failed to quote cetus local state: %w", suiwindow.ErrCoverage)
		}
		return fail("liquidity=insufficient")
	}
	if !totalIn.IsUint64() || !totalOut.IsUint64() || !totalFee.IsUint64() || totalOut.Sign() == 0 {
		return fail("amount=out_of_range")
	}
	return QuoteResult{AmountIn: new(big.Int).Sub(totalIn, totalFee).Uint64(), AmountOut: totalOut.Uint64(), FeeAmount: totalFee.Uint64(), FeeRate: p.FeeRate, AfterSqrtPrice: price}, nil
}

func localDiv(n, d *big.Int, up bool) *big.Int {
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(n, d, r)
	if up && r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	return q
}
func localDelta(a, b, l *big.Int, tokenA, up bool) *big.Int {
	diff := new(big.Int).Sub(a, b)
	diff.Abs(diff)
	n := new(big.Int).Mul(l, diff)
	if tokenA {
		return localDiv(n.Lsh(n, 64), new(big.Int).Mul(a, b), up)
	}
	return localDiv(n, new(big.Int).Lsh(big.NewInt(1), 64), up)
}
func localStep(price, target, l, remaining *big.Int, feeRate uint64, a2b, exactInput bool) (in, out, next, fee *big.Int, err error) {
	denom := new(big.Int).SetUint64(1000000 - feeRate)
	if exactInput {
		available := localDiv(new(big.Int).Mul(remaining, denom), big.NewInt(1000000), false)
		in = localDelta(price, target, l, a2b, true)
		if available.Cmp(in) < 0 {
			in = available
			fee = new(big.Int).Sub(remaining, in)
			next, err = localNext(price, l, in, a2b, true)
		} else {
			next = new(big.Int).Set(target)
			fee = localDiv(new(big.Int).Mul(in, new(big.Int).SetUint64(feeRate)), denom, true)
		}
		if err != nil {
			return
		}
		out = localDelta(price, next, l, !a2b, false)
	} else {
		out = localDelta(price, target, l, !a2b, false)
		if remaining.Cmp(out) < 0 {
			out = new(big.Int).Set(remaining)
			next, err = localNext(price, l, out, a2b, false)
		} else {
			next = new(big.Int).Set(target)
		}
		if err != nil {
			return
		}
		in = localDelta(price, next, l, a2b, true)
		fee = localDiv(new(big.Int).Mul(in, new(big.Int).SetUint64(feeRate)), denom, true)
	}
	return
}
func localNext(price, l, amount *big.Int, a2b, exactInput bool) (*big.Int, error) {
	var next *big.Int
	if a2b == exactInput {
		lq := new(big.Int).Lsh(new(big.Int).Set(l), 64)
		product := new(big.Int).Mul(price, amount)
		d := new(big.Int).Set(lq)
		if exactInput {
			d.Add(d, product)
		} else {
			d.Sub(d, product)
		}
		if d.Sign() <= 0 {
			return nil, fmt.Errorf("failed to calculate cetus next price: denominator=out_of_range")
		}
		next = localDiv(new(big.Int).Mul(lq, price), d, true)
	} else {
		delta := localDiv(new(big.Int).Lsh(new(big.Int).Set(amount), 64), l, !exactInput)
		next = new(big.Int).Set(price)
		if exactInput {
			next.Add(next, delta)
		} else {
			next.Sub(next, delta)
		}
	}
	min, _ := new(big.Int).SetString("4295048016", 10)
	max, _ := new(big.Int).SetString("79226673515401279992447579055", 10)
	if next.Cmp(min) < 0 || next.Cmp(max) > 0 {
		return nil, fmt.Errorf("failed to calculate cetus next price: price=out_of_range")
	}
	return next, nil
}
