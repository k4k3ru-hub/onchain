package solana

import (
	"context"
	"fmt"
	"time"
)

type Transaction struct {
	Signature         Signature
	Slot              Slot
	Timestamp         *time.Time
	Fee               uint64
	Failed            bool
	Logs              []string
	PreTokenBalances  []TokenBalance
	PostTokenBalances []TokenBalance
}

type TokenBalance struct {
	AccountIndex uint16
	Owner        *Address
	Mint         Address
	Amount       string
	Decimals     uint8
}

type transactionProvider interface {
	getTransaction(context.Context, Signature, Commitment) (*Transaction, error)
}

// Transaction returns confirmed Solana transaction details.
func (c *RPCClient) Transaction(ctx context.Context, signature Signature) (*Transaction, error) {
	if c == nil || c.transactionProvider == nil {
		return nil, fmt.Errorf("failed to get solana transaction: transaction_provider=null")
	}
	if signature.IsZero() {
		return nil, fmt.Errorf("failed to get solana transaction: signature=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	transaction, err := c.transactionProvider.getTransaction(ctx, signature, c.config.Commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to get solana transaction: %w", err)
	}
	if transaction == nil {
		return nil, fmt.Errorf("failed to get solana transaction: transaction=null")
	}
	transaction.Logs = append([]string(nil), transaction.Logs...)
	transaction.PreTokenBalances = cloneTokenBalances(transaction.PreTokenBalances)
	transaction.PostTokenBalances = cloneTokenBalances(transaction.PostTokenBalances)
	return transaction, nil
}

func cloneTokenBalances(values []TokenBalance) []TokenBalance {
	result := append([]TokenBalance(nil), values...)
	for i := range result {
		if result[i].Owner != nil {
			value := *result[i].Owner
			result[i].Owner = &value
		}
	}
	return result
}
