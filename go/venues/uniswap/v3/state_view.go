package v3

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const slot0Signature = "slot0()"

type Slot0 struct {
	SqrtPriceX96               *big.Int
	Tick                       int32
	ObservationIndex           uint16
	ObservationCardinality     uint16
	ObservationCardinalityNext uint16
	FeeProtocol                uint8
	Unlocked                   bool
}

// GetSlot0 gets the current Uniswap v3 pool state.
//
// Parameters:
//   - ctx: request context.
//   - poolAddress: configured Uniswap v3 pool address.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Pool Slot0 state.
//   - State retrieval error.
//
// Version:
//   - 2026-08-24: Added.
func (c *HTTPClient) GetSlot0(ctx context.Context, poolAddress common.Address, blockNumber *big.Int) (Slot0, error) {
	if c == nil {
		return Slot0{}, fmt.Errorf("failed to get uniswap v3 slot0: client=null")
	}
	if c.rpc == nil {
		return Slot0{}, fmt.Errorf("failed to get uniswap v3 slot0: http_rpc_client=null")
	}
	if poolAddress == (common.Address{}) {
		return Slot0{}, fmt.Errorf("failed to get uniswap v3 slot0: pool_address=empty")
	}
	if _, ok := configuredPool(c.poolByAdd, poolAddress); !ok {
		return Slot0{}, fmt.Errorf("failed to get uniswap v3 slot0: pool_address=unconfigured")
	}
	if blockNumber != nil && blockNumber.Sign() < 0 {
		return Slot0{}, fmt.Errorf("failed to get uniswap v3 slot0: block_number=out_of_range")
	}
	result, err := c.rpc.CallContract(ctx, ethereum.CallMsg{To: &poolAddress, Data: crypto.Keccak256([]byte(slot0Signature))[:4]}, blockNumber)
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to get uniswap v3 slot0: %w", err)
	}
	slot0, err := decodeSlot0(result)
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to get uniswap v3 slot0: %w", err)
	}
	return slot0, nil
}

func decodeSlot0(result []byte) (Slot0, error) {
	typeNames := []string{"uint160", "int24", "uint16", "uint16", "uint16", "uint8", "bool"}
	arguments := make(abi.Arguments, 0, len(typeNames))
	for _, name := range typeNames {
		t, err := abi.NewType(name, "", nil)
		if err != nil {
			return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: %w: abi_type=%q", err, name)
		}
		arguments = append(arguments, abi.Argument{Type: t})
	}
	values, err := arguments.Unpack(result)
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: %w", err)
	}
	if len(values) != 7 {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: values=invalid actual_length=%d expected_length=7", len(values))
	}
	sqrtPrice, ok := values[0].(*big.Int)
	if !ok || sqrtPrice.Sign() < 0 || sqrtPrice.BitLen() > 160 {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: sqrt_price_x96=invalid")
	}
	tick, ok := values[1].(*big.Int)
	if !ok || !tick.IsInt64() || tick.Int64() < -(1<<23) || tick.Int64() > (1<<23)-1 {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: tick=invalid")
	}
	observationIndex, ok := values[2].(uint16)
	if !ok {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: observation_index=invalid")
	}
	observationCardinality, ok := values[3].(uint16)
	if !ok {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: observation_cardinality=invalid")
	}
	observationCardinalityNext, ok := values[4].(uint16)
	if !ok {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: observation_cardinality_next=invalid")
	}
	feeProtocol, ok := values[5].(uint8)
	if !ok {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: fee_protocol=invalid")
	}
	unlocked, ok := values[6].(bool)
	if !ok {
		return Slot0{}, fmt.Errorf("failed to decode uniswap v3 slot0 result: unlocked=invalid")
	}
	return Slot0{SqrtPriceX96: new(big.Int).Set(sqrtPrice), Tick: int32(tick.Int64()), ObservationIndex: observationIndex, ObservationCardinality: observationCardinality, ObservationCardinalityNext: observationCardinalityNext, FeeProtocol: feeProtocol, Unlocked: unlocked}, nil
}
