package clmm

import (
	"fmt"
	"math/big"
)

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
	d := new(big.Int).Lsh(big.NewInt(1), 64)
	if up {
		n.Add(n, new(big.Int).Rsh(new(big.Int).Set(d), 1))
	}
	return localDiv(n, d, false)
}
func localStep(price, target, l, remaining *big.Int, feeRate uint64, a2b, exactInput bool) (in, out, next, fee *big.Int, err error) {
	denom := new(big.Int).SetUint64(1000000 - feeRate)
	if exactInput {
		available := localDiv(new(big.Int).Mul(remaining, denom), big.NewInt(1000000), false)
		in = localDelta(price, target, l, a2b, true)
		if in.BitLen() > 128 {
			err = fmt.Errorf("failed to calculate turbos step: input=out_of_range")
			return
		}
		if available.Cmp(in) < 0 {
			in = available
			fee = new(big.Int).Sub(remaining, in)
			next, err = localNext(price, l, in, a2b, true)
		} else {
			next = new(big.Int).Set(target)
			fee = localFee(in, feeRate)
		}
		if err != nil {
			return
		}
		in = localDelta(price, next, l, a2b, true)
		if next.Cmp(target) != 0 {
			fee = new(big.Int).Sub(remaining, in)
		} else {
			fee = localFee(in, feeRate)
		}
		out = localDelta(price, next, l, !a2b, false)
	} else {
		out = localDelta(price, target, l, !a2b, false)
		if out.BitLen() > 128 {
			err = fmt.Errorf("failed to calculate turbos step: output=out_of_range")
			return
		}
		if remaining.Cmp(out) < 0 {
			out = new(big.Int).Set(remaining)
			next, err = localNext(price, l, out, a2b, false)
		} else {
			next = new(big.Int).Set(target)
		}
		if err != nil {
			return
		}
		out = localDelta(price, next, l, !a2b, false)
		if out.Cmp(remaining) > 0 {
			out = new(big.Int).Set(remaining)
		}
		in = localDelta(price, next, l, a2b, true)
		fee = localFee(in, feeRate)
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
			return nil, fmt.Errorf("failed to calculate turbos next price: denominator=out_of_range")
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
		return nil, fmt.Errorf("failed to calculate turbos next price: price=out_of_range")
	}
	return next, nil
}

// Turbos uses nearest-integer rounding here, unlike Cetus's ceiling.
func localFee(input *big.Int, rate uint64) *big.Int {
	denominator := new(big.Int).SetUint64(1000000 - rate)
	numerator := new(big.Int).Mul(input, new(big.Int).SetUint64(rate))
	numerator.Add(numerator, new(big.Int).Rsh(new(big.Int).Set(denominator), 1))
	return numerator.Quo(numerator, denominator)
}
