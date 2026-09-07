package protocol

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

const MaxTickSpacing int32 = 1<<23 - 1

type PoolKey struct {
	Token0      Currency
	Token1      Currency
	TickSpacing int32
}

// NewPoolKey creates a canonical Slipstream pool key.
//
// Parameters:
//   - tokenA: First pool currency in any order.
//   - tokenB: Second pool currency in any order.
//   - tickSpacing: Positive int24 tick spacing.
//
// Returns:
//   - Canonically ordered pool key.
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func NewPoolKey(tokenA, tokenB Currency, tickSpacing int32) (PoolKey, error) {
	addressA := tokenA.Address()
	addressB := tokenB.Address()
	if bytes.Compare(addressA[:], addressB[:]) > 0 {
		tokenA, tokenB = tokenB, tokenA
	}
	key := PoolKey{Token0: tokenA, Token1: tokenB, TickSpacing: tickSpacing}
	if err := key.Validate(); err != nil {
		return PoolKey{}, fmt.Errorf("failed to create slipstream pool key: %w", err)
	}
	return key, nil
}

// Validate validates a Slipstream pool key.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func (k PoolKey) Validate() error {
	token0 := k.Token0.Address()
	token1 := k.Token1.Address()
	if token0 == (common.Address{}) {
		return fmt.Errorf("failed to validate slipstream pool key: token0=empty")
	}
	if token1 == (common.Address{}) {
		return fmt.Errorf("failed to validate slipstream pool key: token1=empty")
	}
	if bytes.Compare(token0[:], token1[:]) >= 0 {
		return fmt.Errorf("failed to validate slipstream pool key: token_order=invalid")
	}
	if k.TickSpacing <= 0 || k.TickSpacing > MaxTickSpacing {
		return fmt.Errorf("failed to validate slipstream pool key: tick_spacing=out_of_range min_value=1 max_value=%d", MaxTickSpacing)
	}
	return nil
}
