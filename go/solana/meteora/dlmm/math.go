package dlmm

// Fee and bin conversion formulas conform to Meteora's official commons quote
// implementation at commit fb02e51ae677bbd18e76543f702dae40632426db.

import (
	"fmt"
	"math/big"
)

const (
	basisPointMax = uint64(10_000)
	feePrecision  = uint64(1_000_000_000)
	maxFeeRate    = uint64(100_000_000)
)

var q64 = new(big.Int).Lsh(big.NewInt(1), 64)

func updateFeeReferences(pool *Pool, timestamp int64) error {
	if timestamp < 0 {
		return fmt.Errorf("failed to update meteora dlmm fee references: timestamp=out_of_range min_value=0")
	}
	elapsed := timestamp - pool.VariableParameters.LastUpdateTimestamp
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed >= int64(pool.Parameters.FilterPeriod) {
		pool.VariableParameters.IndexReference = pool.ActiveBinID
		if elapsed < int64(pool.Parameters.DecayPeriod) {
			pool.VariableParameters.VolatilityReference = uint32(
				uint64(pool.VariableParameters.VolatilityAccumulator) * uint64(pool.Parameters.ReductionFactor) / basisPointMax,
			)
		} else {
			pool.VariableParameters.VolatilityReference = 0
		}
	}
	return nil
}

func updateVolatilityAccumulator(pool *Pool) {
	delta := int64(pool.VariableParameters.IndexReference) - int64(pool.ActiveBinID)
	if delta < 0 {
		delta = -delta
	}
	volatility := uint64(pool.VariableParameters.VolatilityReference) + uint64(delta)*basisPointMax
	if volatility > uint64(pool.Parameters.MaxVolatilityAccumulator) {
		volatility = uint64(pool.Parameters.MaxVolatilityAccumulator)
	}
	pool.VariableParameters.VolatilityAccumulator = uint32(volatility)
}

func totalFeeRate(pool Pool) (uint64, error) {
	power := uint64(1)
	for i := uint8(0); i < pool.Parameters.BaseFeePowerFactor; i++ {
		if power > ^uint64(0)/10 {
			return 0, fmt.Errorf("failed to calculate meteora dlmm fee rate: base_fee=out_of_range")
		}
		power *= 10
	}
	base := new(big.Int).SetUint64(uint64(pool.Parameters.BaseFactor))
	base.Mul(base, new(big.Int).SetUint64(uint64(pool.BinStep)))
	base.Mul(base, big.NewInt(10))
	base.Mul(base, new(big.Int).SetUint64(power))
	variable := new(big.Int)
	if pool.Parameters.VariableFeeControl > 0 {
		crossed := new(big.Int).SetUint64(uint64(pool.VariableParameters.VolatilityAccumulator) * uint64(pool.BinStep))
		variable.Mul(crossed, crossed)
		variable.Mul(variable, new(big.Int).SetUint64(uint64(pool.Parameters.VariableFeeControl)))
		variable = divCeil(variable, new(big.Int).SetUint64(100_000_000_000))
	}
	total := new(big.Int).Add(base, variable)
	if total.Cmp(new(big.Int).SetUint64(maxFeeRate)) > 0 {
		return maxFeeRate, nil
	}
	if !total.IsUint64() {
		return 0, fmt.Errorf("failed to calculate meteora dlmm fee rate: fee_rate=out_of_range")
	}
	return total.Uint64(), nil
}

func feeFromIncludedAmount(amount, feeRate uint64) (uint64, error) {
	fee := divCeil(
		new(big.Int).Mul(new(big.Int).SetUint64(amount), new(big.Int).SetUint64(feeRate)),
		new(big.Int).SetUint64(feePrecision),
	)
	if !fee.IsUint64() {
		return 0, fmt.Errorf("failed to calculate meteora dlmm fee: fee=out_of_range")
	}
	return fee.Uint64(), nil
}

func feeFromExcludedAmount(amount, feeRate uint64) (uint64, error) {
	if feeRate >= feePrecision {
		return 0, fmt.Errorf("failed to calculate meteora dlmm fee: fee_rate=out_of_range")
	}
	fee := divCeil(
		new(big.Int).Mul(new(big.Int).SetUint64(amount), new(big.Int).SetUint64(feeRate)),
		new(big.Int).SetUint64(feePrecision-feeRate),
	)
	if !fee.IsUint64() {
		return 0, fmt.Errorf("failed to calculate meteora dlmm fee: fee=out_of_range")
	}
	return fee.Uint64(), nil
}

func amountOutAtBin(amountIn uint64, price *big.Int, swapForY bool) (uint64, error) {
	var result *big.Int
	if swapForY {
		result = new(big.Int).Rsh(new(big.Int).Mul(new(big.Int).SetUint64(amountIn), price), 64)
	} else {
		if price.Sign() == 0 {
			return 0, fmt.Errorf("failed to calculate meteora dlmm amount out: price=empty")
		}
		result = new(big.Int).Div(new(big.Int).Lsh(new(big.Int).SetUint64(amountIn), 64), price)
	}
	if !result.IsUint64() {
		return 0, fmt.Errorf("failed to calculate meteora dlmm amount out: amount_out=out_of_range")
	}
	return result.Uint64(), nil
}

func amountInAtBin(amountOut uint64, price *big.Int, swapForY bool) (uint64, error) {
	var result *big.Int
	if swapForY {
		if price.Sign() == 0 {
			return 0, fmt.Errorf("failed to calculate meteora dlmm amount in: price=empty")
		}
		result = divCeil(new(big.Int).Lsh(new(big.Int).SetUint64(amountOut), 64), price)
	} else {
		result = divCeil(new(big.Int).Mul(new(big.Int).SetUint64(amountOut), price), q64)
	}
	if !result.IsUint64() {
		return 0, fmt.Errorf("failed to calculate meteora dlmm amount in: amount_in=out_of_range")
	}
	return result.Uint64(), nil
}

func uint128LE(value [16]byte) *big.Int {
	reversed := make([]byte, len(value))
	for i := range value {
		reversed[len(value)-1-i] = value[i]
	}
	return new(big.Int).SetBytes(reversed)
}

func divCeil(numerator, denominator *big.Int) *big.Int {
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}
