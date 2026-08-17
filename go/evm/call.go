// call.go
package evm

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
)

type contractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// CallContract calls a read-only EVM contract method using the HTTP RPC client.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - call: generic EVM contract call message.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Raw contract response bytes.
//   - Contract call error.
//
// Version:
//   - 2026-08-17: Added.
func (c *HTTPClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to call evm contract: evm_http_client=null")
	}
	if c.contractCaller == nil {
		return nil, fmt.Errorf("failed to call evm contract: http_eth_client=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	result, err := c.contractCaller.CallContract(ctx, call, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to call evm contract: %w", err)
	}

	return result, nil
}
