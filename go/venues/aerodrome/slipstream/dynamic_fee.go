package slipstream

import (
	"fmt"
	"math/big"
)

// dynamicFeeInputs contains decoded inputs from one retained capture. Oracle
// availability describes the contract's observe result, never missing local data.
// A producer must reject missing local inputs before constructing this value.
type dynamicFeeInputs struct {
	base, spacingFee, cap, defaultCap      uint32
	scaling, defaultScaling                uint64
	initialEnabled                         bool
	initial                                uint32
	timestamp, lastObservation, secondsAgo uint32
	cardinality                            uint16
	tick                                   int32
	cumulativePast, cumulativeNow          int64
	oracleAvailable                        bool
	discount                               uint32 // Discount for the quote call's origin, not the observed trader.
}

// calculateDynamicFee implements DynamicSwapFeeModule.getFee from retained
// configuration and oracle inputs. It has no transport or global dependencies.
func calculateDynamicFee(s dynamicFeeInputs) (uint32, error) {
	if s.base > 30000 || s.initial > 50000 || s.cap > 50000 || s.defaultCap > 50000 || s.scaling > 1e18 || s.defaultScaling > 1e18 || s.discount > 500000 || s.spacingFee > 100000 || s.tick < -887272 || s.tick > 887272 {
		return 0, fmt.Errorf("failed to calculate slipstream dynamic fee: configuration=out_of_range")
	}
	if s.base == 420 {
		return 0, nil
	}
	base := s.base
	if base == 0 {
		base = s.spacingFee
	}
	if s.initialEnabled && s.lastObservation != s.timestamp {
		switch s.initial {
		case 0:
			return base, nil
		case 420:
			return 0, nil
		default:
			return s.initial, nil
		}
	}
	if s.secondsAgo < 2 || s.secondsAgo >= 131070 {
		return 0, fmt.Errorf("failed to calculate slipstream dynamic fee: seconds_ago=out_of_range")
	}
	scaling, cap := s.scaling, s.cap
	if scaling == 0 {
		scaling, cap = s.defaultScaling, s.defaultCap
	}
	fee := new(big.Int).SetUint64(uint64(base))
	if uint32(s.cardinality) >= s.secondsAgo/2 && s.oracleAvailable {
		const limit = int64(1) << 55
		if s.cumulativePast < -limit || s.cumulativePast >= limit || s.cumulativeNow < -limit || s.cumulativeNow >= limit {
			return 0, fmt.Errorf("failed to calculate slipstream dynamic fee: cumulative=out_of_range")
		}
		// Solidity 0.7 int56 subtraction wraps before division, which truncates
		// toward zero. Casting the average back to int24 also wraps.
		delta := s.cumulativeNow - s.cumulativePast
		delta = ((delta + limit) & ((int64(1) << 56) - 1)) - limit
		average := delta / int64(s.secondsAgo)
		average = ((average + (1 << 23)) & ((1 << 24) - 1)) - (1 << 23)
		diff := int64(s.tick) - average
		diff = ((diff + (1 << 23)) & ((1 << 24) - 1)) - (1 << 23)
		if diff < 0 {
			diff = -diff
		}
		dynamic := new(big.Int).Mul(big.NewInt(diff), new(big.Int).SetUint64(scaling))
		dynamic.Quo(dynamic, big.NewInt(1_000_000))
		fee.Add(fee, dynamic)
	}
	if fee.Cmp(new(big.Int).SetUint64(uint64(cap))) > 0 {
		fee.SetUint64(uint64(cap))
	}
	total := fee.Uint64()
	// The contract rounds the discount up, then subtracts it.
	discount := (total*uint64(s.discount) + 999999) / 1_000_000
	return uint32(total - discount), nil
}
