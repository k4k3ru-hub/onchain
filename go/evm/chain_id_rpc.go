package evm

import (
	"context"
	"fmt"
	"math/big"
)

type chainIDProvider interface {
	ChainID(ctx context.Context) (*big.Int, error)
}

// ChainID returns the EVM chain ID reported by the RPC endpoint.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//
// Returns:
//   - EVM chain ID.
//   - Retrieval error.
//
// Version:
//   - 2026-08-20: Added.
func (c *HTTPClient) ChainID(ctx context.Context) (ChainID, error) {
	if c == nil {
		return 0, fmt.Errorf("failed to get evm chain id: http_client=null")
	}
	if c.chainIDProvider == nil {
		return 0, fmt.Errorf("failed to get evm chain id: chain_id_provider=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	chainID, err := c.chainIDProvider.ChainID(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get evm chain id: %w", err)
	}
	if chainID == nil {
		return 0, fmt.Errorf("failed to get evm chain id: chain_id=null")
	}
	if !chainID.IsUint64() {
		return 0, fmt.Errorf("failed to get evm chain id: chain_id=out_of_range")
	}

	return ChainID(chainID.Uint64()), nil
}
