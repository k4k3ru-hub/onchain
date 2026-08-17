// block_number.go
package evm

import (
	"context"
	"fmt"
)

type blockNumberer interface {
	BlockNumber(ctx context.Context) (uint64, error)
}

// Get latest block number.
//
// Version:
//   - 2026-05-24: Added.
func (c *HTTPClient) BlockNumber(ctx context.Context) (uint64, error) {
	// Guard.
	if c == nil {
		return 0, fmt.Errorf("failed to get latest block number: evm_http_client=null")
	}
	if c.blockNumberer == nil {
		return 0, fmt.Errorf("failed to get latest block number: missing required parameter: http_eth_client=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	blockNumber, err := c.blockNumberer.BlockNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest block number: %w", err)
	}

	return blockNumber, nil
}
