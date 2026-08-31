package clmm

// The integer formulas in this file conform to Raydium CLMM on-chain program
// commit ed7c84a54ced59c55981780546adb0b4583dcf85. The public API/IDL is the
// primary contract; the program's tick_math, liquidity_math, sqrt_price_math,
// and swap_math modules define rounding behavior not documented by the API.

import (
	"fmt"
	"math/big"
)

const (
	feeRateDenominator = uint64(1_000_000)
	minTick            = int32(-443636)
	maxTick            = int32(443636)
)

var (
	q64             = new(big.Int).Lsh(big.NewInt(1), 64)
	maxUint128      = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	tickMultipliers = [...]uint64{
		0xfffcb933bd6fb800, 0xfff97272373d4000, 0xfff2e50f5f657000,
		0xffe5caca7e10f000, 0xffcb9843d60f7000, 0xff973b41fa98e800,
		0xff2ea16466c9b000, 0xfe5dee046a9a3800, 0xfcbe86c7900bb000,
		0xf987a7253ac65800, 0xf3392b0822bb6000, 0xe7159475a2caf000,
		0xd097f3bdfd2f2000, 0xa9f746462d9f8000, 0x70d869a156f31c00,
		0x31be135f97ed3200, 0x09aa508b5b85a500, 0x005d6af8dedc582c,
		0x00002216e584f5fa,
	}
)

type swapStep struct {
	sqrtPriceNext *big.Int
	amountIn      uint64
	amountOut     uint64
	feeAmount     uint64
}

func sqrtPriceAtTick(tick int32) (*big.Int, error) {
	if tick < minTick || tick > maxTick {
		return nil, fmt.Errorf("failed to calculate raydium clmm sqrt price: tick=out_of_range min_value=%d max_value=%d", minTick, maxTick)
	}
	absTick := uint32(tick)
	if tick < 0 {
		absTick = uint32(-tick)
	}
	ratio := new(big.Int).Set(q64)
	for bit, multiplier := range tickMultipliers {
		if absTick&(1<<bit) != 0 {
			ratio.Mul(ratio, new(big.Int).SetUint64(multiplier))
			ratio.Rsh(ratio, 64)
		}
	}
	if tick > 0 {
		ratio.Div(new(big.Int).Set(maxUint128), ratio)
	}
	return ratio, nil
}

func tickAtSqrtPrice(price *big.Int) (int32, error) {
	minimum, _ := sqrtPriceAtTick(minTick)
	maximum, _ := sqrtPriceAtTick(maxTick)
	if price.Cmp(minimum) < 0 || price.Cmp(maximum) >= 0 {
		return 0, fmt.Errorf("failed to calculate raydium clmm tick: sqrt_price=out_of_range")
	}
	low, high := minTick, maxTick
	for low < high {
		middle := low + (high-low+1)/2
		middlePrice, _ := sqrtPriceAtTick(middle)
		if middlePrice.Cmp(price) <= 0 {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return low, nil
}

func deltaAmounts(a, b, liquidity *big.Int, zeroForOne bool) (uint64, uint64, error) {
	if a.Sign() <= 0 || b.Sign() <= 0 || liquidity.Sign() < 0 {
		return 0, 0, fmt.Errorf("failed to calculate raydium clmm delta amounts: value=invalid")
	}
	low, high := new(big.Int).Set(a), new(big.Int).Set(b)
	if low.Cmp(high) > 0 {
		low, high = high, low
	}
	amount1X64 := new(big.Int).Mul(liquidity, new(big.Int).Sub(high, low))
	product := new(big.Int).Mul(low, high)
	var amountIn, amountOut *big.Int
	if zeroForOne {
		amountIn = divCeil(new(big.Int).Lsh(new(big.Int).Set(amount1X64), 64), product)
		amountOut = new(big.Int).Rsh(new(big.Int).Set(amount1X64), 64)
	} else {
		amountIn = divCeil(amount1X64, q64)
		amountOut = new(big.Int).Div(new(big.Int).Lsh(new(big.Int).Set(amount1X64), 64), product)
	}
	if !amountIn.IsUint64() || !amountOut.IsUint64() {
		return 0, 0, fmt.Errorf("failed to calculate raydium clmm delta amounts: amount=out_of_range")
	}
	return amountIn.Uint64(), amountOut.Uint64(), nil
}

func nextSqrtPriceFromInput(current, liquidity *big.Int, amount uint64, zeroForOne bool) (*big.Int, error) {
	if liquidity.Sign() <= 0 {
		return nil, fmt.Errorf("failed to calculate raydium clmm next sqrt price: liquidity=empty")
	}
	if amount == 0 {
		return new(big.Int).Set(current), nil
	}
	if zeroForOne {
		numerator := new(big.Int).Lsh(new(big.Int).Set(liquidity), 64)
		denominator := new(big.Int).Add(new(big.Int).Set(numerator), new(big.Int).Mul(new(big.Int).SetUint64(amount), current))
		return divCeil(new(big.Int).Mul(numerator, current), denominator), nil
	}
	quotient := new(big.Int).Div(new(big.Int).Lsh(new(big.Int).SetUint64(amount), 64), liquidity)
	return new(big.Int).Add(current, quotient), nil
}

func computeExactInputStep(current, target, liquidity *big.Int, amountRemaining uint64, feeRate uint32, zeroForOne, feeOnInput bool) (swapStep, error) {
	if uint64(feeRate) >= feeRateDenominator {
		return swapStep{}, fmt.Errorf("failed to calculate raydium clmm swap step: fee_rate=out_of_range max_value=%d", feeRateDenominator-1)
	}
	amountForPrice := amountRemaining
	if feeOnInput {
		amountForPrice = mulDivFloor64(amountRemaining, feeRateDenominator-uint64(feeRate), feeRateDenominator)
	}
	maxIn, maxOut, maxErr := deltaAmounts(target, current, liquidity, zeroForOne)
	reachesTarget := maxErr == nil && amountForPrice >= maxIn
	result := swapStep{sqrtPriceNext: new(big.Int)}
	if reachesTarget {
		result.sqrtPriceNext.Set(target)
		result.amountIn, result.amountOut = maxIn, maxOut
	} else {
		next, err := nextSqrtPriceFromInput(current, liquidity, amountForPrice, zeroForOne)
		if err != nil {
			return swapStep{}, err
		}
		result.sqrtPriceNext.Set(next)
		result.amountIn, result.amountOut, maxErr = deltaAmounts(next, current, liquidity, zeroForOne)
		if maxErr != nil {
			return swapStep{}, maxErr
		}
	}
	if feeOnInput {
		if !reachesTarget {
			if result.amountIn > amountRemaining {
				return swapStep{}, fmt.Errorf("failed to calculate raydium clmm swap step: amount_in=out_of_range")
			}
			result.feeAmount = amountRemaining - result.amountIn
		} else {
			result.feeAmount = mulDivCeil64(result.amountIn, uint64(feeRate), feeRateDenominator-uint64(feeRate))
		}
	} else {
		result.feeAmount = mulDivCeil64(result.amountOut, uint64(feeRate), feeRateDenominator)
		if result.feeAmount > result.amountOut {
			return swapStep{}, fmt.Errorf("failed to calculate raydium clmm swap step: fee_amount=out_of_range")
		}
		result.amountOut -= result.feeAmount
		if !reachesTarget {
			result.amountIn = amountRemaining
		}
	}
	return result, nil
}

func divCeil(numerator, denominator *big.Int) *big.Int {
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func mulDivFloor64(value, multiplier, denominator uint64) uint64 {
	result := new(big.Int).Mul(new(big.Int).SetUint64(value), new(big.Int).SetUint64(multiplier))
	result.Div(result, new(big.Int).SetUint64(denominator))
	return result.Uint64()
}

func mulDivCeil64(value, multiplier, denominator uint64) uint64 {
	result := divCeil(
		new(big.Int).Mul(new(big.Int).SetUint64(value), new(big.Int).SetUint64(multiplier)),
		new(big.Int).SetUint64(denominator),
	)
	return result.Uint64()
}
