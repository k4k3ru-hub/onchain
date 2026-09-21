package v4

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/poolfee"
)

// CombinePoolFees combines a fixed LP rate and packed directional protocol rates.
// Hook charges and per-swap overrides are outside this hook-free calculation.
//
// Version:
//   - 2026-09-22: Added.
func CombinePoolFees(lpFee, protocolFee uint32) (poolfee.Rates, error) {
	if lpFee > 1_000_000 || protocolFee>>24 != 0 || protocolFee&0xfff > 1000 || protocolFee>>12 > 1000 {
		return poolfee.Rates{}, fmt.Errorf("failed to calculate uniswap v4 fees: rate=out_of_range")
	}
	combine := func(p uint32) uint32 { return p + lpFee - uint32(uint64(p)*uint64(lpFee)/1_000_000) }
	return poolfee.Rates{Token0ToToken1: combine(protocolFee & 0xfff), Token1ToToken0: combine(protocolFee >> 12)}, nil
}

// DecodeProtocolFee decodes a ProtocolFeeUpdated log for a verified PoolManager.
// The caller owns ordering, removals and Pool ID matching.
//
// Version:
//   - 2026-09-22: Added.
func DecodeProtocolFee(log types.Log) (uint32, error) {
	if log.Removed || len(log.Topics) != 2 || len(log.Data) != 32 || log.Topics[0] != crypto.Keccak256Hash([]byte("ProtocolFeeUpdated(bytes32,uint24)")) {
		return 0, fmt.Errorf("failed to decode uniswap v4 protocol fee: event=invalid")
	}
	v, err := poolfee.UintWord(log.Data, (1<<24)-1)
	if err != nil {
		return 0, fmt.Errorf("failed to decode uniswap v4 protocol fee: %w", err)
	}
	if _, err := CombinePoolFees(0, uint32(v)); err != nil {
		return 0, fmt.Errorf("failed to decode uniswap v4 protocol fee: %w", err)
	}
	return uint32(v), nil
}

// ReadPoolFees reads hook-free fixed-LP-fee Pool rates from an explicit StateView block.
// The caller supplies the verified PoolKey LP fee and deployment's StateView.
//
// Version:
//   - 2026-09-22: Added.
func ReadPoolFees(ctx context.Context, rpc poolfee.ContractReader, stateView common.Address, poolID common.Hash, hooks common.Address, lpFee uint32, block *big.Int) (poolfee.Rates, error) {
	if hooks != (common.Address{}) || lpFee > 1_000_000 {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v4 fees: %w", poolfee.ErrUnsupported)
	}
	if poolID == (common.Hash{}) {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v4 fees: pool_id=empty")
	}
	raw, err := poolfee.Call(ctx, rpc, stateView, common.Address{}, block, "getSlot0(bytes32)", poolID[:])
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v4 fees: %w", err)
	}
	if len(raw) != 128 || new(big.Int).SetBytes(raw[:32]).Sign() == 0 || new(big.Int).SetBytes(raw[:32]).BitLen() > 160 {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v4 fees: slot0=invalid")
	}
	protocol, err := poolfee.UintWord(raw[64:96], (1<<24)-1)
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v4 fees: %w", err)
	}
	lp, err := poolfee.UintWord(raw[96:], 1_000_000)
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v4 fees: %w", err)
	}
	if uint32(lp) != lpFee {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v4 fees: lp_fee=mismatch")
	}
	rates, err := CombinePoolFees(lpFee, uint32(protocol))
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v4 fees: %w", err)
	}
	return rates, nil
}
