package clliquidity

import (
	"fmt"
	"math/big"
)

// PositionPrincipal calculates unrounded token base units for one price range.
// Rational values retain sub-unit principal when comparing protected positions;
// they are not a quote for the integer amounts returned by decreaseLiquidity.
// Inputs are not mutated.
//
// Version:
//   - 2026-09-23: Added.
func PositionPrincipal(price, liquidity *big.Int, lower, upper int32) (token0, token1 *big.Rat, err error) {
	if price == nil || liquidity == nil {
		return nil, nil, fmt.Errorf("failed to calculate position principal: input=null")
	}
	if lower < -887272 || upper > 887272 || lower >= upper || liquidity.Sign() < 0 || liquidity.BitLen() > 128 || price.Cmp(sqrtAtTick(-887272)) < 0 || price.Cmp(sqrtAtTick(887272)) >= 0 {
		return nil, nil, fmt.Errorf("failed to calculate position principal: input=out_of_range")
	}
	a, b, c := sqrtAtTick(lower), sqrtAtTick(upper), new(big.Int).Set(price)
	if c.Cmp(a) < 0 {
		c.Set(a)
	}
	if c.Cmp(b) > 0 {
		c.Set(b)
	}
	n0 := new(big.Int).Mul(liquidity, new(big.Int).Sub(b, c))
	n0.Lsh(n0, 96)
	n1 := new(big.Int).Mul(liquidity, new(big.Int).Sub(c, a))
	return new(big.Rat).SetFrac(n0, new(big.Int).Mul(c, b)), new(big.Rat).SetFrac(n1, power2(96)), nil
}
