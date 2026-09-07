package slipstream

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
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

// DecodeSwapLog decodes a Slipstream pool Swap event log.
//
// Parameters:
//   - eventLog: EVM event log.
//
// Returns:
//   - Decoded Swap without a PoolKey association.
//   - Decode error.
//
// Version:
//   - 2026-08-30: Added.
func DecodeSwapLog(eventLog types.Log) (Swap, error) {
	if eventLog.Address == (common.Address{}) {
		return Swap{}, fmt.Errorf("failed to decode slipstream swap log: pool_address=empty")
	}
	if len(eventLog.Topics) != 3 {
		return Swap{}, fmt.Errorf("failed to decode slipstream swap log: topics=invalid actual_length=%d expected_length=3", len(eventLog.Topics))
	}
	if eventLog.Topics[0] != swapEventSignatureHash() {
		return Swap{}, fmt.Errorf("failed to decode slipstream swap log: event_signature=invalid")
	}
	sender, err := decodeIndexedAddress(eventLog.Topics[1], "sender")
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode slipstream swap log: %w", err)
	}
	recipient, err := decodeIndexedAddress(eventLog.Topics[2], "recipient")
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode slipstream swap log: %w", err)
	}
	amount0, amount1, sqrtPriceX96, liquidity, tick, err := decodeSwapData(eventLog.Data)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to decode slipstream swap log: %w", err)
	}
	return Swap{
		PoolAddress:      eventLog.Address,
		Sender:           sender,
		Recipient:        recipient,
		Amount0:          amount0,
		Amount1:          amount1,
		SqrtPriceX96:     sqrtPriceX96,
		Liquidity:        liquidity,
		Tick:             tick,
		BlockNumber:      eventLog.BlockNumber,
		BlockHash:        eventLog.BlockHash,
		TransactionHash:  eventLog.TxHash,
		TransactionIndex: eventLog.TxIndex,
		LogIndex:         eventLog.Index,
		Removed:          eventLog.Removed,
	}, nil
}

func swapEventSignatureHash() common.Hash {
	return crypto.Keccak256Hash([]byte(swapEventSignature))
}

func decodeIndexedAddress(topic common.Hash, name string) (common.Address, error) {
	if !bytes.Equal(topic[:12], make([]byte, 12)) {
		return common.Address{}, fmt.Errorf("failed to decode indexed address: %s_topic=invalid", name)
	}
	return common.BytesToAddress(topic[12:]), nil
}

func decodeSwapData(data []byte) (*big.Int, *big.Int, *big.Int, *big.Int, int32, error) {
	const expectedLength = 32 * 5
	if len(data) != expectedLength {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode slipstream swap data: data_length=invalid actual_length=%d expected_length=%d", len(data), expectedLength)
	}
	amount0 := decodeInt256Word(data[0:32])
	amount1 := decodeInt256Word(data[32:64])
	sqrtPriceX96, err := decodeUnsignedWord(data[64:96], 160, "sqrt_price_x96")
	if err != nil {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode slipstream swap data: %w", err)
	}
	liquidity, err := decodeUnsignedWord(data[96:128], 128, "liquidity")
	if err != nil {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode slipstream swap data: %w", err)
	}
	tick, err := decodeInt24Word(data[128:160], "tick")
	if err != nil {
		return nil, nil, nil, nil, 0, fmt.Errorf("failed to decode slipstream swap data: %w", err)
	}
	return amount0, amount1, sqrtPriceX96, liquidity, tick, nil
}

func decodeInt256Word(word []byte) *big.Int {
	value := new(big.Int).SetBytes(word)
	if len(word) == 32 && word[0]&0x80 != 0 {
		value.Sub(value, new(big.Int).Lsh(big.NewInt(1), 256))
	}
	return value
}
