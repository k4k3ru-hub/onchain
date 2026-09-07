package v3

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
)

const swapEventSignature = "Swap(address,address,int256,int256,uint160,uint128,int24)"

type Swap struct {
	PoolAddress      common.Address
	PoolKey          protocol.PoolKey
	Sender           common.Address
	Recipient        common.Address
	Amount0          *big.Int
	Amount1          *big.Int
	SqrtPriceX96     *big.Int
	Liquidity        *big.Int
	Tick             int32
	BlockNumber      uint64
	BlockHash        common.Hash
	TransactionHash  common.Hash
	TransactionIndex uint
	LogIndex         uint
	Removed          bool
}

// DecodeSwapLog decodes a Uniswap v3 pool Swap event log.
//
// Parameters:
//   - eventLog: EVM event log.
//
// Returns:
//   - Decoded Swap event without a configured PoolKey association.
//   - Decode error.
//
// Version:
//   - 2026-08-24: Added.
func DecodeSwapLog(eventLog types.Log) (Swap, error) {
	if eventLog.Address == (common.Address{}) {
		return Swap{}, fmt.Errorf("failed to decode uniswap v3 swap log: pool_address=empty")
	}
	if len(eventLog.Topics) != 3 {
		return Swap{}, fmt.Errorf("failed to decode uniswap v3 swap log: topics=invalid actual_length=%d expected_length=3", len(eventLog.Topics))
	}
	if eventLog.Topics[0] != swapEventSignatureHash() {
		return Swap{}, fmt.Errorf("failed to decode uniswap v3 swap log: event_signature=invalid")
	}
	sender, err := indexedAddress(eventLog.Topics[1], "sender")
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode uniswap v3 swap log: %w", err)
	}
	recipient, err := indexedAddress(eventLog.Topics[2], "recipient")
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode uniswap v3 swap log: %w", err)
	}
	amount0, amount1, sqrtPrice, liquidity, tick, err := decodeSwapData(eventLog.Data)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode uniswap v3 swap log: %w", err)
	}
	return Swap{PoolAddress: eventLog.Address, Sender: sender, Recipient: recipient, Amount0: amount0, Amount1: amount1, SqrtPriceX96: sqrtPrice, Liquidity: liquidity, Tick: tick, BlockNumber: eventLog.BlockNumber, BlockHash: eventLog.BlockHash, TransactionHash: eventLog.TxHash, TransactionIndex: eventLog.TxIndex, LogIndex: eventLog.Index, Removed: eventLog.Removed}, nil
}

func swapEventSignatureHash() common.Hash { return crypto.Keccak256Hash([]byte(swapEventSignature)) }

func indexedAddress(topic common.Hash, name string) (common.Address, error) {
	if !bytes.Equal(topic[:12], make([]byte, 12)) {
		return common.Address{}, fmt.Errorf("failed to decode indexed address: %s_topic=invalid", name)
	}
	return common.BytesToAddress(topic[12:]), nil
}

func decodeSwapData(data []byte) (*big.Int, *big.Int, *big.Int, *big.Int, int32, error) {
	if len(data) != 160 {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: data=invalid actual_length=%d expected_length=160", len(data))
	}
	if !hasCanonicalSignedPadding(data[0:32], 256) || !hasCanonicalSignedPadding(data[32:64], 256) {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: amounts=invalid")
	}
	if !hasCanonicalUnsignedPadding(data[64:96], 160) {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: sqrt_price_x96=invalid")
	}
	if !hasCanonicalUnsignedPadding(data[96:128], 128) {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: liquidity=invalid")
	}
	if !hasCanonicalSignedPadding(data[128:160], 24) {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: tick=invalid")
	}
	args := make(abi.Arguments, 0, 5)
	for _, name := range []string{"int256", "int256", "uint160", "uint128", "int24"} {
		t, err := abi.NewType(name, "", nil)
		if err != nil {
			return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: %w", err)
		}
		args = append(args, abi.Argument{Type: t})
	}
	values, err := args.Unpack(data)
	if err != nil {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: %w", err)
	}
	amount0, ok := values[0].(*big.Int)
	if !ok {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: amount0=invalid")
	}
	amount1, ok := values[1].(*big.Int)
	if !ok {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: amount1=invalid")
	}
	sqrtPrice, ok := values[2].(*big.Int)
	if !ok {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: sqrt_price_x96=invalid")
	}
	liquidity, ok := values[3].(*big.Int)
	if !ok {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: liquidity=invalid")
	}
	tick, ok := values[4].(*big.Int)
	if !ok || !tick.IsInt64() {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode swap event data: tick=invalid")
	}
	return new(big.Int).Set(amount0), new(big.Int).Set(amount1), new(big.Int).Set(sqrtPrice), new(big.Int).Set(liquidity), int32(tick.Int64()), nil
}

func hasCanonicalUnsignedPadding(word []byte, bitSize int) bool {
	valueLength := bitSize / 8
	return len(word) == 32 && bytes.Equal(word[:32-valueLength], make([]byte, 32-valueLength))
}

func hasCanonicalSignedPadding(word []byte, bitSize int) bool {
	if len(word) != 32 {
		return false
	}
	start := 32 - bitSize/8
	padding := byte(0)
	if word[start]&0x80 != 0 {
		padding = 0xff
	}
	for _, value := range word[:start] {
		if value != padding {
			return false
		}
	}
	return true
}
