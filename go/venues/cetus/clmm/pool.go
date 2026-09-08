package clmm

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type Pool struct {
	Address          onchainSui.Address
	InitialVersion   uint64
	CoinTypeA        string
	CoinTypeB        string
	CoinA            *big.Int
	CoinB            *big.Int
	CurrentSqrtPrice *big.Int
	CurrentTickIndex int32
	Liquidity        *big.Int
	FeeRate          uint64
	Paused           bool
}

type poolJSON struct {
	CoinA            json.RawMessage `json:"coin_a"`
	CoinB            json.RawMessage `json:"coin_b"`
	CurrentSqrtPrice json.RawMessage `json:"current_sqrt_price"`
	CurrentTickIndex json.RawMessage `json:"current_tick_index"`
	Liquidity        json.RawMessage `json:"liquidity"`
	FeeRate          json.RawMessage `json:"fee_rate"`
	IsPause          bool            `json:"is_pause"`
}

// ParsePool parses a Cetus CLMM pool from a Sui Move object.
//
// Parameters:
//   - object: Sui pool object.
//
// Returns:
//   - Parsed pool.
//   - Parse or validation error.
//
// Version:
//   - 2026-08-30: Added.
func ParsePool(object *onchainSui.Object) (*Pool, error) {
	if object == nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: object=null")
	}
	if object.Move == nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: move_object=null")
	}
	coinTypeA, coinTypeB, err := parsePoolType(object.Move.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: %w", err)
	}
	var value poolJSON
	if err := json.Unmarshal(object.Move.JSON, &value); err != nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: failed to decode move object: %w", err)
	}
	coinA, err := jsonUnsigned(value.CoinA)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: coin_a=invalid: %w", err)
	}
	coinB, err := jsonUnsigned(value.CoinB)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: coin_b=invalid: %w", err)
	}
	sqrtPrice, err := jsonUnsigned(value.CurrentSqrtPrice)
	if err != nil || sqrtPrice.Sign() <= 0 {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: current_sqrt_price=invalid")
	}
	liquidity, err := jsonUnsigned(value.Liquidity)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: liquidity=invalid: %w", err)
	}
	feeRate, err := jsonUint64(value.FeeRate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: fee_rate=invalid: %w", err)
	}
	tick, err := jsonSigned32(value.CurrentTickIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus clmm pool: current_tick_index=invalid: %w", err)
	}
	return &Pool{
		Address: object.Address, InitialVersion: object.Version, CoinTypeA: coinTypeA, CoinTypeB: coinTypeB,
		CoinA: coinA, CoinB: coinB, CurrentSqrtPrice: sqrtPrice, CurrentTickIndex: tick,
		Liquidity: liquidity, FeeRate: feeRate, Paused: value.IsPause,
	}, nil
}

func parsePoolType(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	marker := "::pool::Pool<"
	index := strings.Index(value, marker)
	if index < 0 || !strings.HasSuffix(value, ">") {
		return "", "", fmt.Errorf("pool_type=invalid")
	}
	arguments := strings.TrimSuffix(value[index+len(marker):], ">")
	parts := strings.Split(arguments, ",")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("pool_type=invalid")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func jsonUnsigned(value json.RawMessage) (*big.Int, error) {
	text := strings.Trim(strings.TrimSpace(string(value)), `"`)
	if text == "" || text == "null" {
		return nil, fmt.Errorf("value=empty")
	}
	result, ok := new(big.Int).SetString(text, 10)
	if !ok || result.Sign() < 0 {
		return nil, fmt.Errorf("value=invalid")
	}
	return result, nil
}

func jsonUint64(value json.RawMessage) (uint64, error) {
	parsed, err := jsonUnsigned(value)
	if err != nil || !parsed.IsUint64() {
		return 0, fmt.Errorf("value=invalid")
	}
	return parsed.Uint64(), nil
}

func jsonSigned32(value json.RawMessage) (int32, error) {
	var wrapped struct {
		Bits json.RawMessage `json:"bits"`
	}
	if err := json.Unmarshal(value, &wrapped); err == nil && len(wrapped.Bits) > 0 {
		value = wrapped.Bits
	}
	text := strings.Trim(strings.TrimSpace(string(value)), `"`)
	parsed, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return 0, fmt.Errorf("value=invalid")
	}
	if parsed.IsUint64() && parsed.Uint64() <= uint64(^uint32(0)) {
		return int32(uint32(parsed.Uint64())), nil
	}
	if parsed.IsInt64() && parsed.Int64() >= -1<<31 && parsed.Int64() <= 1<<31-1 {
		return int32(parsed.Int64()), nil
	}
	return 0, fmt.Errorf("value=out_of_range")
}
