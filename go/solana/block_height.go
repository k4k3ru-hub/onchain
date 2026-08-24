package solana

import (
	"context"
	"fmt"
	"strconv"
)

type BlockHeight uint64

type blockHeightProvider interface {
	getBlockHeight(ctx context.Context, commitment Commitment) (BlockHeight, error)
}

// Uint64 returns the numeric Solana block height.
//
// Returns:
//   - Numeric block height.
//
// Version:
//   - 2026-08-22: Added.
func (h BlockHeight) Uint64() uint64 {
	return uint64(h)
}

// String returns the decimal Solana block height.
//
// Returns:
//   - Decimal block height.
//
// Version:
//   - 2026-08-22: Added.
func (h BlockHeight) String() string {
	return strconv.FormatUint(h.Uint64(), 10)
}

// Validate validates a Solana block height.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-22: Added.
func (h BlockHeight) Validate() error {
	if h == 0 {
		return fmt.Errorf("failed to validate solana block height: block_height=empty")
	}
	return nil
}

// BlockHeight returns the current Solana block height at the configured commitment.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//
// Returns:
//   - Current Solana block height.
//   - Retrieval error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) BlockHeight(ctx context.Context) (BlockHeight, error) {
	if c == nil {
		return 0, fmt.Errorf("failed to get solana block height: rpc_client=null")
	}
	if c.blockHeightProvider == nil {
		return 0, fmt.Errorf("failed to get solana block height: block_height_provider=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	blockHeight, err := c.blockHeightProvider.getBlockHeight(ctx, c.config.Commitment)
	if err != nil {
		return 0, fmt.Errorf("failed to get solana block height: %w", err)
	}
	if err := blockHeight.Validate(); err != nil {
		return 0, fmt.Errorf("failed to get solana block height: %w", err)
	}
	return blockHeight, nil
}
