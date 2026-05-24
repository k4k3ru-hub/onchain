//
// block_number.go
//
package evm

import (
    "context"
    "fmt"
)


//
// Get latest block number.
//
// Version:
//   - 2026-05-24: Added.
//
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
    // Guard.
    if c == nil {
        return 0, fmt.Errorf("failed to get latest block number: missing required parameter: evm_client=null")
    }
    if c.httpETHClient == nil {
        return 0, fmt.Errorf("failed to get latest block number: missing required parameter: http_eth_client=null")
    }
    if ctx == nil {
        ctx = context.Background()
    }

    blockNumber, err := c.httpETHClient.BlockNumber(ctx)
    if err != nil {
        return 0, fmt.Errorf("failed to get latest block number: %w", err)
    }

    return blockNumber, nil
}
