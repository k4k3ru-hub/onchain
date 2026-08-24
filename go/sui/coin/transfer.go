package coin

import (
	"context"
	"fmt"
	"math/big"
	"time"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type Transfer struct {
	Transaction onchainSui.TransactionDigest
	Checkpoint  *onchainSui.CheckpointSequenceNumber
	Timestamp   *time.Time
	From        onchainSui.Address
	To          onchainSui.Address
	CoinType    string
	Amount      *big.Int
}

type TransferQuery struct {
	Address          onchainSui.Address
	CoinType         string
	First            int
	After            string
	AfterCheckpoint  *onchainSui.CheckpointSequenceNumber
	AtCheckpoint     *onchainSui.CheckpointSequenceNumber
	BeforeCheckpoint *onchainSui.CheckpointSequenceNumber
}

type TransferPage struct {
	Transfers   []Transfer
	HasNextPage bool
	NextCursor  string
}

// TransfersByTransaction returns configured coin transfers from transaction effects.
func (c *Client) TransfersByTransaction(ctx context.Context, digest onchainSui.TransactionDigest) ([]Transfer, error) {
	if c == nil || c.provider == nil {
		return nil, fmt.Errorf("failed to get sui coin transfers by transaction: client=null")
	}
	if digest.IsZero() {
		return nil, fmt.Errorf("failed to get sui coin transfers by transaction: digest=empty")
	}
	effects, err := c.provider.TransactionEffects(ctx, digest)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui coin transfers by transaction: %w", err)
	}
	if effects == nil {
		return nil, fmt.Errorf("failed to get sui coin transfers by transaction: effects=null")
	}
	if !effects.Successful {
		return []Transfer{}, nil
	}
	return c.deriveTransfers(effects, ""), nil
}

// Transfers returns historical configured coin transfers affecting an address.
func (c *Client) Transfers(ctx context.Context, query TransferQuery) (TransferPage, error) {
	if c == nil || c.provider == nil {
		return TransferPage{}, fmt.Errorf("failed to get sui coin transfers: client=null")
	}
	if query.Address.IsZero() {
		return TransferPage{}, fmt.Errorf("failed to get sui coin transfers: address=empty")
	}
	selectedCoinType := ""
	if query.CoinType != "" {
		var err error
		selectedCoinType, err = onchainSui.NormalizeMoveType(query.CoinType)
		if err != nil || !c.hasCoinType(selectedCoinType) {
			return TransferPage{}, fmt.Errorf("failed to get sui coin transfers: coin_type=not_configured")
		}
	}
	page, err := c.provider.TransactionDigests(ctx, onchainSui.TransactionQuery{
		AffectedAddress: query.Address, First: query.First, After: query.After,
		AfterCheckpoint: query.AfterCheckpoint, AtCheckpoint: query.AtCheckpoint, BeforeCheckpoint: query.BeforeCheckpoint,
	})
	if err != nil {
		return TransferPage{}, fmt.Errorf("failed to get sui coin transfers: %w", err)
	}
	transfers := make([]Transfer, 0)
	for _, digest := range page.Digests {
		effects, err := c.provider.TransactionEffects(ctx, digest)
		if err != nil {
			return TransferPage{}, fmt.Errorf("failed to get sui coin transfers: %w", err)
		}
		if effects == nil || !effects.Successful {
			continue
		}
		for _, transfer := range c.deriveTransfers(effects, selectedCoinType) {
			if transfer.From == query.Address || transfer.To == query.Address {
				transfers = append(transfers, transfer)
			}
		}
	}
	return TransferPage{Transfers: transfers, HasNextPage: page.HasNextPage, NextCursor: page.NextCursor}, nil
}

func (c *Client) deriveTransfers(effects *onchainSui.TransactionEffects, selectedCoinType string) []Transfer {
	type delta struct {
		address onchainSui.Address
		amount  *big.Int
	}
	negative := make(map[string][]delta)
	positive := make(map[string][]delta)
	for _, change := range effects.BalanceChanges {
		if !c.hasCoinType(change.CoinType) || (selectedCoinType != "" && change.CoinType != selectedCoinType) || change.Amount == nil || change.Amount.Sign() == 0 {
			continue
		}
		value := new(big.Int).Set(change.Amount)
		if value.Sign() < 0 {
			value.Neg(value)
			negative[change.CoinType] = append(negative[change.CoinType], delta{address: change.Address, amount: value})
		} else {
			positive[change.CoinType] = append(positive[change.CoinType], delta{address: change.Address, amount: value})
		}
	}
	transfers := make([]Transfer, 0)
	for coinType, senders := range negative {
		recipients := positive[coinType]
		senderIndex, recipientIndex := 0, 0
		for senderIndex < len(senders) && recipientIndex < len(recipients) {
			amount := new(big.Int).Set(senders[senderIndex].amount)
			if recipients[recipientIndex].amount.Cmp(amount) < 0 {
				amount.Set(recipients[recipientIndex].amount)
			}
			transfers = append(transfers, Transfer{
				Transaction: effects.Digest, Checkpoint: effects.Checkpoint, Timestamp: effects.Timestamp,
				From: senders[senderIndex].address, To: recipients[recipientIndex].address, CoinType: coinType, Amount: amount,
			})
			senders[senderIndex].amount.Sub(senders[senderIndex].amount, amount)
			recipients[recipientIndex].amount.Sub(recipients[recipientIndex].amount, amount)
			if senders[senderIndex].amount.Sign() == 0 {
				senderIndex++
			}
			if recipients[recipientIndex].amount.Sign() == 0 {
				recipientIndex++
			}
		}
	}
	return transfers
}
