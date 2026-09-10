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

type transactionSender interface {
	SendTransaction(ctx context.Context, transaction *types.Transaction) error
}

// SendTransaction sends a signed EVM transaction using the HTTP RPC client.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - transaction: signed EVM transaction.
//
// Returns:
//   - Submitted transaction hash.
//   - Validation or RPC submission error.
//
// Version:
//   - 2026-09-10: Added.
func (c *HTTPClient) SendTransaction(ctx context.Context, transaction *types.Transaction) (common.Hash, error) {
	if c == nil {
		return common.Hash{}, fmt.Errorf("failed to send evm transaction: evm_http_client=null")
	}
	if c.transactionSender == nil {
		return common.Hash{}, fmt.Errorf("failed to send evm transaction: http_eth_client=null")
	}
	if transaction == nil {
		return common.Hash{}, fmt.Errorf("failed to send evm transaction: transaction=null")
	}
	_, signatureR, signatureS := transaction.RawSignatureValues()
	if signatureR == nil || signatureS == nil || signatureR.Sign() == 0 || signatureS.Sign() == 0 {
		return common.Hash{}, fmt.Errorf("failed to send evm transaction: transaction=unsigned")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return common.Hash{}, fmt.Errorf("failed to send evm transaction: %w", err)
	}
	if err := c.transactionSender.SendTransaction(ctx, transaction); err != nil {
		return common.Hash{}, fmt.Errorf("failed to send evm transaction: %w", err)
	}
	return transaction.Hash(), nil
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
