package v3

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// FilterSwaps gets historical Swap events for the configured Uniswap v3 pools.
//
// Parameters:
//   - ctx: request context.
//   - fromBlock: first block to query; nil uses the RPC default.
//   - toBlock: last block to query; nil uses the latest block.
//
// Returns:
//   - Decoded Swap events in RPC response order.
//   - Filter or decode error.
//
// Version:
//   - 2026-08-24: Added.
func (c *HTTPClient) FilterSwaps(ctx context.Context, fromBlock, toBlock *big.Int) ([]Swap, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to filter uniswap v3 swaps: client=null")
	}
	if c.rpc == nil {
		return nil, fmt.Errorf("failed to filter uniswap v3 swaps: http_rpc_client=null")
	}
	logs, err := c.rpc.FilterLogs(ctx, ethereum.FilterQuery{FromBlock: fromBlock, ToBlock: toBlock, Addresses: poolAddresses(c.poolByAdd), Topics: [][]common.Hash{{swapEventSignatureHash()}}})
	if err != nil {
		return nil, fmt.Errorf("failed to filter uniswap v3 swaps: %w", err)
	}
	swaps := make([]Swap, 0, len(logs))
	for i, eventLog := range logs {
		poolKey, ok := configuredPool(c.poolByAdd, eventLog.Address)
		if !ok {
			return nil, fmt.Errorf("failed to filter uniswap v3 swaps: pool_address=unconfigured: result_index=%d", i)
		}
		swap, err := DecodeSwapLog(eventLog)
		if err != nil {
			return nil, fmt.Errorf("failed to filter uniswap v3 swaps: %w: result_index=%d", err, i)
		}
		swap.PoolKey = poolKey
		swaps = append(swaps, swap)
	}
	return swaps, nil
}
