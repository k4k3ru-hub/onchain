package v3

import (
	"fmt"
	"math/big"
)

// Q96 arithmetic follows the integer rounding specified by Uniswap v3-core
// TickMath, SqrtPriceMath and SwapMath. No floating-point prices are used.
func power2(n uint) *big.Int { return new(big.Int).Lsh(big.NewInt(1), n) }
func ceilDiv(n, d *big.Int) *big.Int {
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(n, d, r)
	if r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	return q
}
func sqrtAtTick(tick int32) *big.Int {
	factors := [...]string{"fffcb933bd6fad37aa2d162d1a594001", "fff97272373d413259a46990580e213a", "fff2e50f5f656932ef12357cf3c7fdcc", "ffe5caca7e10e4e61c3624eaa0941cd0", "ffcb9843d60f6159c9db58835c926644", "ff973b41fa98c081472e6896dfb254c0", "ff2ea16466c96a3843ec78b326b52861", "fe5dee046a99a2a811c461f1969c3053", "fcbe86c7900a88aedcffc83b479aa3a4", "f987a7253ac413176f2b074cf7815e54", "f3392b0822b70005940c7a398e4b70f3", "e7159475a2c29b7443b29c7fa6e889d9", "d097f3bdfd2022b8845ad8f792aa5825", "a9f746462d870fdf8a65dc1f90e061e5", "70d869a156d2a1b890bb3df62baf32f7", "31be135f97d08fd981231505542fcfa6", "9aa508b5b7a84e1c677de54f3e99bc9", "5d6af8dedb81196699c329225ee604", "2216e584f5fa1ea926041bedfe98", "48a170391f7dc42444e8fa2"}
	abs := tick
	if abs < 0 {
		abs = -abs
	}
	ratio := power2(128)
	for bit, hex := range factors {
		if abs&(1<<bit) != 0 {
			f, _ := new(big.Int).SetString(hex, 16)
			ratio.Rsh(new(big.Int).Mul(ratio, f), 128)
		}
	}
	if tick > 0 {
		ratio.Quo(new(big.Int).Sub(power2(256), big.NewInt(1)), ratio)
	}
	return ceilDiv(ratio, power2(32))
}
func amountDelta(a, b, l *big.Int, token0, up bool) *big.Int {
	if a.Cmp(b) > 0 {
		a, b = b, a
	}
	n := new(big.Int).Mul(l, new(big.Int).Sub(b, a))
	d := power2(96)
	if token0 {
		n.Lsh(n, 96)
		d.Mul(a, b)
	}
	if up {
		return ceilDiv(n, d)
	}
	return n.Quo(n, d)
}
func nextPrice(p, l, a *big.Int, zero, input bool) (*big.Int, error) {
	if l.Sign() == 0 {
		return nil, fmt.Errorf("failed to calculate uniswap v3 next price: liquidity=empty")
	}
	if zero == input {
		n := new(big.Int).Lsh(new(big.Int).Set(l), 96)
		product := new(big.Int).Mul(a, p)
		d := new(big.Int).Set(n)
		if input {
			d.Add(d, product)
		} else {
			d.Sub(d, product)
		}
		if d.Sign() <= 0 {
			return nil, fmt.Errorf("failed to calculate uniswap v3 next price: denominator=out_of_range")
		}
		// Match the uint256-overflow fallback used for token0 exact input.
		if input && (product.BitLen() > 256 || d.BitLen() > 256) {
			return ceilDiv(n, new(big.Int).Add(new(big.Int).Quo(n, p), a)), nil
		}
		return ceilDiv(new(big.Int).Mul(n, p), d), nil
	}
	n := new(big.Int).Lsh(new(big.Int).Set(a), 96)
	if input {
		return new(big.Int).Add(p, n.Quo(n, l)), nil
	}
	result := new(big.Int).Sub(p, ceilDiv(n, l))
	if result.Sign() <= 0 {
		return nil, fmt.Errorf("failed to calculate uniswap v3 next price: sqrt_price=out_of_range")
	}
	return result, nil
}

type localStep struct{ price, in, out, fee *big.Int }

func calculateStep(p, target, l, remaining *big.Int, fee uint32, input, zero bool) (localStep, error) {
	available := new(big.Int).Set(remaining)
	if input {
		available.Quo(new(big.Int).Mul(remaining, big.NewInt(int64(1000000-fee))), big.NewInt(1000000))
	}
	needed := amountDelta(p, target, l, zero == input, input)
	next := new(big.Int).Set(target)
	if available.Cmp(needed) < 0 {
		var err error
		next, err = nextPrice(p, l, available, zero, input)
		if err != nil {
			return localStep{}, err
		}
	}
	in := amountDelta(p, next, l, zero, true)
	out := amountDelta(p, next, l, !zero, false)
	if !input && out.Cmp(remaining) > 0 {
		out.Set(remaining)
	}
	var charged *big.Int
	if input && next.Cmp(target) != 0 {
		charged = new(big.Int).Sub(remaining, in)
	} else {
		charged = ceilDiv(new(big.Int).Mul(in, big.NewInt(int64(fee))), big.NewInt(int64(1000000-fee)))
	}
	return localStep{next, in, out, charged}, nil
}
