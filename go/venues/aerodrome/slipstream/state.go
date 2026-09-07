package slipstream

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

const (
	slot0Signature       = "slot0()"
	liquiditySignature   = "liquidity()"
	tickSpacingSignature = "tickSpacing()"
)

type PoolStateClientParams struct {
	RPC HTTPRPCClient
}

type PoolStateClient struct {
	rpc HTTPRPCClient
}

type Slot0 struct {
	SqrtPriceX96               *big.Int
	Tick                       int32
	ObservationIndex           uint16
	ObservationCardinality     uint16
	ObservationCardinalityNext uint16
	Unlocked                   bool
}

// NewPoolStateClient creates a Slipstream pool-state read client.
//
// Parameters:
//   - params: RPC dependency.
//
// Returns:
//   - Pool-state read client.
//   - Client creation error.
//
// Version:
//   - 2026-08-30: Added.
func NewPoolStateClient(params PoolStateClientParams) (*PoolStateClient, error) {
	if params.RPC == nil {
		return nil, fmt.Errorf("failed to create slipstream pool state client: http_rpc_client=null")
	}
	return &PoolStateClient{rpc: params.RPC}, nil
}

// GetSlot0 gets the mutable state packed in a Slipstream pool's slot0.
//
// Parameters:
//   - ctx: Request context.
//   - pool: Slipstream pool contract address.
//   - blockNumber: State block; nil uses latest.
//
// Returns:
//   - Decoded slot0 state.
//   - State retrieval error.
//
// Version:
//   - 2026-08-30: Added.
func (c *PoolStateClient) GetSlot0(ctx context.Context, pool common.Address, blockNumber *big.Int) (Slot0, error) {
	response, err := c.callPool(ctx, pool, slot0Signature, blockNumber)
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to get slipstream slot0: %w", err)
	}
	state, err := decodeSlot0(response)
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to get slipstream slot0: %w", err)
	}
	return state, nil
}

// GetLiquidity gets the currently active in-range pool liquidity.
//
// Parameters:
//   - ctx: Request context.
//   - pool: Slipstream pool contract address.
//   - blockNumber: State block; nil uses latest.
//
// Returns:
//   - Active uint128 liquidity.
//   - State retrieval error.
//
// Version:
//   - 2026-08-30: Added.
func (c *PoolStateClient) GetLiquidity(ctx context.Context, pool common.Address, blockNumber *big.Int) (*big.Int, error) {
	response, err := c.callPool(ctx, pool, liquiditySignature, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get slipstream liquidity: %w", err)
	}
	liquidity, err := decodeUnsignedWord(response, 128, "liquidity")
	if err != nil {
		return nil, fmt.Errorf("failed to get slipstream liquidity: %w", err)
	}
	return liquidity, nil
}

// GetTickSpacing gets a Slipstream pool's immutable tick spacing.
//
// Parameters:
//   - ctx: Request context.
//   - pool: Slipstream pool contract address.
//   - blockNumber: State block; nil uses latest.
//
// Returns:
//   - Positive int24 tick spacing.
//   - State retrieval error.
//
// Version:
//   - 2026-08-30: Added.
func (c *PoolStateClient) GetTickSpacing(ctx context.Context, pool common.Address, blockNumber *big.Int) (int32, error) {
	response, err := c.callPool(ctx, pool, tickSpacingSignature, blockNumber)
	if err != nil {
		return 0, fmt.Errorf("failed to get slipstream tick spacing: %w", err)
	}
	tickSpacing, err := decodeInt24Word(response, "tick_spacing")
	if err != nil {
		return 0, fmt.Errorf("failed to get slipstream tick spacing: %w", err)
	}
	if tickSpacing <= 0 {
		return 0, fmt.Errorf("failed to get slipstream tick spacing: tick_spacing=out_of_range min_value=1")
	}
	return tickSpacing, nil
}

func (c *PoolStateClient) callPool(ctx context.Context, pool common.Address, signature string, blockNumber *big.Int) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to call slipstream pool state: pool_state_client=null")
	}
	if pool == (common.Address{}) {
		return nil, fmt.Errorf("failed to call slipstream pool state: pool=empty")
	}
	if err := validateBlockNumber(blockNumber); err != nil {
		return nil, fmt.Errorf("failed to call slipstream pool state: %w", err)
	}
	response, err := c.rpc.CallContract(ctx, ethereum.CallMsg{To: &pool, Data: methodSelector(signature)}, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to call slipstream pool contract: %w", err)
	}
	return response, nil
}

func decodeSlot0(response []byte) (Slot0, error) {
	const expectedLength = 32 * 6
	if len(response) != expectedLength {
		return Slot0{}, fmt.Errorf("failed to decode slipstream slot0 result: response_length=invalid actual_length=%d expected_length=%d", len(response), expectedLength)
	}
	sqrtPriceX96, err := decodeUnsignedWord(response[0:32], 160, "sqrt_price_x96")
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to decode slipstream slot0 result: %w", err)
	}
	tick, err := decodeInt24Word(response[32:64], "tick")
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to decode slipstream slot0 result: %w", err)
	}
	observationIndex, err := decodeUint16Word(response[64:96], "observation_index")
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to decode slipstream slot0 result: %w", err)
	}
	observationCardinality, err := decodeUint16Word(response[96:128], "observation_cardinality")
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to decode slipstream slot0 result: %w", err)
	}
	observationCardinalityNext, err := decodeUint16Word(response[128:160], "observation_cardinality_next")
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to decode slipstream slot0 result: %w", err)
	}
	unlocked, err := decodeBoolWord(response[160:192], "unlocked")
	if err != nil {
		return Slot0{}, fmt.Errorf("failed to decode slipstream slot0 result: %w", err)
	}
	return Slot0{
		SqrtPriceX96:               sqrtPriceX96,
		Tick:                       tick,
		ObservationIndex:           observationIndex,
		ObservationCardinality:     observationCardinality,
		ObservationCardinalityNext: observationCardinalityNext,
		Unlocked:                   unlocked,
	}, nil
}

func decodeUnsignedWord(word []byte, bits int, name string) (*big.Int, error) {
	if len(word) != 32 {
		return nil, fmt.Errorf("failed to decode unsigned word: %s=invalid response_length=%d expected_length=32", name, len(word))
	}
	value := new(big.Int).SetBytes(word)
	if value.BitLen() > bits {
		return nil, fmt.Errorf("failed to decode unsigned word: %s=out_of_range", name)
	}
	return value, nil
}

func decodeUint16Word(word []byte, name string) (uint16, error) {
	value, err := decodeUnsignedWord(word, 16, name)
	if err != nil {
		return 0, err
	}
	return uint16(value.Uint64()), nil
}

func decodeInt24Word(word []byte, name string) (int32, error) {
	if len(word) != 32 {
		return 0, fmt.Errorf("failed to decode int24 word: %s=invalid response_length=%d expected_length=32", name, len(word))
	}
	negative := word[29]&0x80 != 0
	wantPrefix := byte(0)
	if negative {
		wantPrefix = 0xff
	}
	for _, value := range word[:29] {
		if value != wantPrefix {
			return 0, fmt.Errorf("failed to decode int24 word: %s=invalid", name)
		}
	}
	raw := uint32(word[29])<<16 | uint32(word[30])<<8 | uint32(word[31])
	if negative {
		return int32(raw) - 1<<24, nil
	}
	return int32(raw), nil
}

func decodeBoolWord(word []byte, name string) (bool, error) {
	value, err := decodeUnsignedWord(word, 8, name)
	if err != nil {
		return false, err
	}
	if value.Uint64() > 1 {
		return false, fmt.Errorf("failed to decode bool word: %s=invalid", name)
	}
	return value.Sign() == 1, nil
}
