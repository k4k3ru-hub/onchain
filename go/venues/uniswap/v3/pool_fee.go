package v3

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/poolfee"
)

// DecodePoolCreatedFee extracts the immutable swap fee from a verified factory log.
// Factory identity and canonicality must be checked by the caller.
//
// Version:
//   - 2026-09-22: Added.
func DecodePoolCreatedFee(log types.Log) (poolfee.Rates, error) {
	if log.Removed || len(log.Topics) != 4 || len(log.Data) != 64 || log.Topics[0] != crypto.Keccak256Hash([]byte("PoolCreated(address,address,uint24,int24,address)")) {
		return poolfee.Rates{}, fmt.Errorf("failed to decode uniswap v3 fee: event=invalid")
	}
	v, err := poolfee.UintWord(log.Topics[3][:], 999999)
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to decode uniswap v3 fee: %w", err)
	}
	return poolfee.Rates{Token0ToToken1: uint32(v), Token1ToToken0: uint32(v)}, nil
}

// ReadPoolFee reads the immutable Pool fee at an explicit block.
// The caller must verify the Pool's factory and creation evidence.
//
// Version:
//   - 2026-09-22: Added.
func ReadPoolFee(ctx context.Context, rpc poolfee.ContractReader, pool common.Address, block *big.Int) (poolfee.Rates, error) {
	raw, err := poolfee.Call(ctx, rpc, pool, common.Address{}, block, "fee()", nil)
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v3 fee: %w", err)
	}
	v, err := poolfee.UintWord(raw, 999999)
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read uniswap v3 fee: %w", err)
	}
	return poolfee.Rates{Token0ToToken1: uint32(v), Token1ToToken0: uint32(v)}, nil
}
