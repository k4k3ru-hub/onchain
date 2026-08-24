package spltoken

import (
	"context"
	"fmt"
	"math/big"
	"time"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type TransferFilter struct {
	Address onchainSolana.Address
	Mint    *onchainSolana.Address
	Limit   int
	Cursor  string
	MinSlot *onchainSolana.Slot
	MaxSlot *onchainSolana.Slot
}

type TransferEvent struct {
	Signature onchainSolana.Signature
	Slot      onchainSolana.Slot
	Timestamp *time.Time
	From      onchainSolana.Address
	To        onchainSolana.Address
	Mint      onchainSolana.Address
	Amount    string
	Decimals  uint8
}

type TransferProvider interface {
	TransferEvents(context.Context, TransferFilter) ([]TransferEvent, error)
}

type TransferPage struct {
	Events     []TransferEvent
	NextCursor string
}

type TransferPageProvider interface {
	TransferEventPage(context.Context, TransferFilter) (*TransferPage, error)
}

type transactionSource interface {
	SignaturesForAddress(context.Context, onchainSolana.Address, int) ([]onchainSolana.Signature, error)
	Transaction(context.Context, onchainSolana.Signature) (*onchainSolana.Transaction, error)
}

type StandardTransferProvider struct{ source transactionSource }

// NewStandardTransferProvider creates a transfer provider backed by standard Solana RPC.
func NewStandardTransferProvider(source transactionSource) (*StandardTransferProvider, error) {
	if source == nil {
		return nil, fmt.Errorf("failed to create standard transfer provider: transaction_source=null")
	}
	return &StandardTransferProvider{source: source}, nil
}

// TransferEvents returns SPL Token transfers from the RPC node's retained transaction history.
func (p *StandardTransferProvider) TransferEvents(ctx context.Context, filter TransferFilter) ([]TransferEvent, error) {
	if p == nil || p.source == nil {
		return nil, fmt.Errorf("failed to get spl token transfers: transaction_source=null")
	}
	if filter.Address.IsZero() {
		return nil, fmt.Errorf("failed to get spl token transfers: address=empty")
	}
	if filter.Limit < 1 || filter.Limit > 1000 {
		return nil, fmt.Errorf("failed to get spl token transfers: limit=out_of_range min_value=1 max_value=1000")
	}
	signatures, err := p.source.SignaturesForAddress(ctx, filter.Address, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get spl token transfers: %w", err)
	}
	var events []TransferEvent
	for _, signature := range signatures {
		transaction, err := p.source.Transaction(ctx, signature)
		if err != nil {
			return nil, fmt.Errorf("failed to get spl token transfers: %w", err)
		}
		events = append(events, deriveTransfers(transaction, filter.Mint)...)
	}
	return events, nil
}

// TransferEvents returns SPL Token transfers using the configured provider.
func (c *Client) TransferEvents(ctx context.Context, filter TransferFilter) ([]TransferEvent, error) {
	if c == nil || c.transferProvider == nil {
		return nil, fmt.Errorf("failed to get spl token transfers: transfer_provider=null")
	}
	return c.transferProvider.TransferEvents(ctx, filter)
}

// TransferEventPage returns a page of SPL Token transfers and an optional provider cursor.
func (c *Client) TransferEventPage(ctx context.Context, filter TransferFilter) (*TransferPage, error) {
	if c == nil || c.transferProvider == nil {
		return nil, fmt.Errorf("failed to get spl token transfer page: transfer_provider=null")
	}
	if provider, ok := c.transferProvider.(TransferPageProvider); ok {
		return provider.TransferEventPage(ctx, filter)
	}
	events, err := c.transferProvider.TransferEvents(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get spl token transfer page: %w", err)
	}
	return &TransferPage{Events: events}, nil
}

type balanceDelta struct {
	owner, mint onchainSolana.Address
	amount      *big.Int
	decimals    uint8
}

func deriveTransfers(transaction *onchainSolana.Transaction, mintFilter *onchainSolana.Address) []TransferEvent {
	if transaction == nil || transaction.Failed {
		return nil
	}
	type key struct {
		index uint16
		mint  onchainSolana.Address
	}
	balances := map[key]balanceDelta{}
	apply := func(values []onchainSolana.TokenBalance, sign int64) {
		for _, value := range values {
			if value.Owner == nil || (mintFilter != nil && value.Mint != *mintFilter) {
				continue
			}
			amount, ok := new(big.Int).SetString(value.Amount, 10)
			if !ok {
				continue
			}
			amount.Mul(amount, big.NewInt(sign))
			k := key{value.AccountIndex, value.Mint}
			delta := balances[k]
			if delta.amount == nil {
				delta = balanceDelta{owner: *value.Owner, mint: value.Mint, amount: new(big.Int), decimals: value.Decimals}
			}
			delta.amount.Add(delta.amount, amount)
			balances[k] = delta
		}
	}
	apply(transaction.PreTokenBalances, -1)
	apply(transaction.PostTokenBalances, 1)
	var negatives, positives []balanceDelta
	for _, value := range balances {
		if value.amount.Sign() < 0 {
			value.amount.Abs(value.amount)
			negatives = append(negatives, value)
		} else if value.amount.Sign() > 0 {
			positives = append(positives, value)
		}
	}
	var events []TransferEvent
	for i := range negatives {
		for j := range positives {
			if negatives[i].mint != positives[j].mint || negatives[i].amount.Sign() == 0 || positives[j].amount.Sign() == 0 {
				continue
			}
			amount := new(big.Int).Set(negatives[i].amount)
			if amount.Cmp(positives[j].amount) > 0 {
				amount.Set(positives[j].amount)
			}
			events = append(events, TransferEvent{Signature: transaction.Signature, Slot: transaction.Slot, Timestamp: transaction.Timestamp, From: negatives[i].owner, To: positives[j].owner, Mint: negatives[i].mint, Amount: amount.String(), Decimals: negatives[i].decimals})
			negatives[i].amount.Sub(negatives[i].amount, amount)
			positives[j].amount.Sub(positives[j].amount, amount)
		}
	}
	return events
}

// ParseTransactionTransfers parses SPL Token transfers from transaction balance changes.
//
// Parameters:
//   - transaction: Solana transaction details.
//   - mint: optional mint filter.
//
// Returns:
//   - Parsed transfer events.
//
// Version:
//   - 2026-08-22: Added.
func ParseTransactionTransfers(transaction *onchainSolana.Transaction, mint *onchainSolana.Address) []TransferEvent {
	return deriveTransfers(transaction, mint)
}
