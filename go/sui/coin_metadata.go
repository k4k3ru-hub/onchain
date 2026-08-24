package sui

import (
	"context"
	"fmt"
	"math/big"
	"strings"
)

type CoinMetadata struct {
	CoinType         string
	MetadataAddress  Address
	Decimals         uint8
	Name             string
	Symbol           string
	Description      string
	IconURL          string
	Supply           *big.Int
	RegulatedState   string
	AllowGlobalPause bool
	SupplyState      string
}

// CoinMetadata returns metadata for a Sui coin type.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - coinType: Move coin type.
//
// Returns:
//   - SDK-owned coin metadata.
//   - Retrieval or validation error.
//
// Version:
//   - 2026-08-23: Added.
func (c *RPCClient) CoinMetadata(ctx context.Context, coinType string) (*CoinMetadata, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get sui coin metadata: rpc_client=null")
	}
	if c.caller == nil {
		return nil, fmt.Errorf("failed to get sui coin metadata: coin_metadata_provider=null")
	}
	normalizedCoinType, err := normalizeMovePath(coinType)
	if err != nil || !strings.Contains(normalizedCoinType, "::") {
		return nil, fmt.Errorf("failed to get sui coin metadata: coin_type=invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result struct {
		CoinMetadata *struct {
			Address          string `json:"address"`
			Decimals         int    `json:"decimals"`
			Name             string `json:"name"`
			Symbol           string `json:"symbol"`
			Description      string `json:"description"`
			IconURL          string `json:"iconUrl"`
			Supply           string `json:"supply"`
			RegulatedState   string `json:"regulatedState"`
			AllowGlobalPause bool   `json:"allowGlobalPause"`
			SupplyState      string `json:"supplyState"`
		} `json:"coinMetadata"`
	}
	query := fmt.Sprintf(`query { coinMetadata(coinType: %q) { address decimals name symbol description iconUrl supply regulatedState allowGlobalPause supplyState } }`, normalizedCoinType)
	if err := c.caller.query(ctx, query, &result); err != nil {
		return nil, fmt.Errorf("failed to get sui coin metadata: %w", err)
	}
	if result.CoinMetadata == nil {
		return nil, fmt.Errorf("failed to get sui coin metadata: coin_metadata=null")
	}
	value := result.CoinMetadata
	if value.Decimals < 0 || value.Decimals > 255 {
		return nil, fmt.Errorf("failed to get sui coin metadata: decimals=out_of_range")
	}
	if strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.Symbol) == "" {
		return nil, fmt.Errorf("failed to get sui coin metadata: identity=invalid")
	}
	address, err := ParseAddress(value.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui coin metadata: %w", err)
	}
	supply, ok := new(big.Int).SetString(value.Supply, 10)
	if !ok || supply.Sign() < 0 {
		return nil, fmt.Errorf("failed to get sui coin metadata: supply=invalid")
	}
	return &CoinMetadata{
		CoinType: normalizedCoinType, MetadataAddress: address, Decimals: uint8(value.Decimals),
		Name: value.Name, Symbol: value.Symbol, Description: value.Description, IconURL: value.IconURL,
		Supply: supply, RegulatedState: value.RegulatedState, AllowGlobalPause: value.AllowGlobalPause, SupplyState: value.SupplyState,
	}, nil
}
