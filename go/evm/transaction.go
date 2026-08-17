// transaction.go
package evm

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type transactionReceiptProvider interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// TransactionReceipt gets an EVM transaction receipt using the HTTP RPC client.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - txHash: transaction hash.
//
// Returns:
//   - EVM transaction receipt.
//   - Receipt retrieval error.
//
// Version:
//   - 2026-08-17: Added.
func (c *HTTPClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get evm transaction receipt: evm_http_client=null")
	}
	if c.receiptProvider == nil {
		return nil, fmt.Errorf("failed to get evm transaction receipt: http_eth_client=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	receipt, err := c.receiptProvider.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get evm transaction receipt: %w", err)
	}

	return receipt, nil
}
