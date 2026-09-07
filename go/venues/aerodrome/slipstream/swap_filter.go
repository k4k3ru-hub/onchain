package slipstream

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

type LogFilterRPCClient interface {
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
}

type SwapFilterClientParams struct {
	RPC     LogFilterRPCClient
	Sources []SwapSource
}

type SwapFilterClient struct {
	rpc         LogFilterRPCClient
	poolKeys    map[common.Address]protocol.PoolKey
	poolAddress []common.Address
}

// NewSwapFilterClient creates a configured historical Swap filter client.
//
// Parameters:
//   - params: Log-filter RPC dependency and pool sources.
//
// Returns:
//   - Historical Swap filter client.
//   - Client creation error.
//
// Version:
//   - 2026-08-30: Added.
func NewSwapFilterClient(params SwapFilterClientParams) (*SwapFilterClient, error) {
	if params.RPC == nil {
		return nil, fmt.Errorf("failed to create slipstream swap filter client: log_filter_rpc_client=null")
	}
	poolKeys, poolAddresses, err := buildSwapSources(params.Sources)
	if err != nil {
		return nil, fmt.Errorf("failed to create slipstream swap filter client: %w", err)
	}
	return &SwapFilterClient{rpc: params.RPC, poolKeys: poolKeys, poolAddress: poolAddresses}, nil
}

// FilterSwaps gets historical Swap events for configured Slipstream pools.
//
// Parameters:
//   - ctx: Request context.
//   - fromBlock: First block to query; nil uses the RPC default.
//   - toBlock: Last block to query; nil uses latest.
//
// Returns:
//   - Decoded Swap events in RPC response order.
//   - Filter or decode error.
//
// Version:
//   - 2026-08-30: Added.
func (c *SwapFilterClient) FilterSwaps(ctx context.Context, fromBlock, toBlock *big.Int) ([]Swap, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to filter slipstream swaps: swap_filter_client=null")
	}
	if err := validateBlockRange(fromBlock, toBlock); err != nil {
		return nil, fmt.Errorf("failed to filter slipstream swaps: %w", err)
	}
	logs, err := c.rpc.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: append([]common.Address(nil), c.poolAddress...),
		Topics:    [][]common.Hash{{swapEventSignatureHash()}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to filter slipstream swaps: %w", err)
	}
	swaps := make([]Swap, 0, len(logs))
	for index, eventLog := range logs {
		poolKey, exists := c.poolKeys[eventLog.Address]
		if !exists {
			return nil, fmt.Errorf("failed to filter slipstream swaps: pool_address=unconfigured result_index=%d", index)
		}
		swap, err := DecodeSwapLog(eventLog)
		if err != nil {
			return nil, fmt.Errorf("failed to filter slipstream swaps: %w: result_index=%d", err, index)
		}
		swap.PoolKey = poolKey
		swaps = append(swaps, swap)
	}
	return swaps, nil
}

func validateBlockRange(fromBlock, toBlock *big.Int) error {
	if err := validateBlockNumber(fromBlock); err != nil {
		return fmt.Errorf("failed to validate block range: %w: boundary=from", err)
	}
	if err := validateBlockNumber(toBlock); err != nil {
		return fmt.Errorf("failed to validate block range: %w: boundary=to", err)
	}
	if fromBlock != nil && toBlock != nil && fromBlock.Cmp(toBlock) > 0 {
		return fmt.Errorf("failed to validate block range: block_range=invalid")
	}
	return nil
}
