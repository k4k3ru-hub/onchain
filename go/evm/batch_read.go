package evm

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

type batchCaller interface {
	BatchCallContext(context.Context, []rpc.BatchElem) error
}

// ReadContracts batches read-only calls to one contract at an explicit block number.
// Results and per-call errors preserve input order. A transport error is returned
// separately; callers must reject partial results when any error is present.
// The batch contains at most 64 calls and never retries individual failures.
//
// Version:
//   - 2026-09-08: Added.
func (c *HTTPClient) ReadContracts(ctx context.Context, target common.Address, data [][]byte, block uint64) ([][]byte, []error, error) {
	if c == nil || c.batchCaller == nil {
		return nil, nil, fmt.Errorf("failed to batch read evm contracts: rpc=null")
	}
	if target == (common.Address{}) || len(data) == 0 || len(data) > 64 {
		return nil, nil, fmt.Errorf("failed to batch read evm contracts: parameters=invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	values := make([]hexutil.Bytes, len(data))
	elems := make([]rpc.BatchElem, len(data))
	for i, input := range data {
		elems[i] = rpc.BatchElem{Method: "eth_call", Args: []any{map[string]any{"to": target, "data": hexutil.Encode(input)}, hexutil.EncodeUint64(block)}, Result: &values[i]}
	}
	if err := c.batchCaller.BatchCallContext(ctx, elems); err != nil {
		return nil, nil, fmt.Errorf("failed to batch read evm contracts: %w", err)
	}
	results := make([][]byte, len(data))
	failures := make([]error, len(data))
	for i, elem := range elems {
		results[i] = values[i]
		if elem.Error != nil {
			failures[i] = fmt.Errorf("failed to read evm contract: %w: call_index=%d", elem.Error, i)
		}
	}
	return results, failures, nil
}
