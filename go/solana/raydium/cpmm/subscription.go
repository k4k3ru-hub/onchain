package cpmm

import (
	"context"
	"fmt"
	"time"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
	"github.com/k4k3ru-hub/onchain/go/solana/internal/ammtransaction"
)

type LogSubscription interface {
	Recv(context.Context) (*onchainSolana.Log, error)
	Close()
}
type SubscribeLogsFunc func(onchainSolana.Address) (LogSubscription, error)
type transactionSource interface {
	Transaction(context.Context, onchainSolana.Signature) (*onchainSolana.Transaction, error)
}

type SwapEvent struct {
	Signature                   onchainSolana.Signature
	Slot                        onchainSolana.Slot
	Timestamp                   time.Time
	Pool, InputMint, OutputMint onchainSolana.Address
	AmountIn, AmountOut         uint64
	EventIndex                  uint32
}

type SwapSubscriber struct {
	source    transactionSource
	subscribe SubscribeLogsFunc
	pools     map[onchainSolana.Address]Pool
}
type SwapSubscription struct {
	source transactionSource
	logs   LogSubscription
	pool   Pool
}

// NewSwapSubscriber creates a Raydium CPMM swap subscriber for discovered pools.
//
// Parameters:
//   - client: Raydium CPMM client containing discovered pools.
//   - source: confirmed transaction source.
//   - subscribe: Solana log subscription function.
//
// Returns:
//   - Swap subscriber.
//   - Creation error.
//
// Version:
//   - 2026-09-01: Added.
func NewSwapSubscriber(client *Client, source transactionSource, subscribe SubscribeLogsFunc) (*SwapSubscriber, error) {
	if client == nil || source == nil || subscribe == nil {
		return nil, fmt.Errorf("failed to create raydium cpmm swap subscriber: dependency=null")
	}
	pools := make(map[onchainSolana.Address]Pool, len(client.pools))
	for address, pool := range client.pools {
		pools[address] = pool
	}
	return &SwapSubscriber{source: source, subscribe: subscribe, pools: pools}, nil
}

// SubscribeSwaps subscribes to successful transactions mentioning a configured pool.
//
// Parameters:
//   - ctx: subscription context.
//   - poolAddress: configured pool address.
//
// Returns:
//   - Swap subscription.
//   - Subscription error.
//
// Version:
//   - 2026-09-01: Added.
func (s *SwapSubscriber) SubscribeSwaps(_ context.Context, poolAddress onchainSolana.Address) (*SwapSubscription, error) {
	if s == nil || s.source == nil || s.subscribe == nil {
		return nil, fmt.Errorf("failed to subscribe raydium cpmm swaps: subscriber=null")
	}
	pool, ok := s.pools[poolAddress]
	if !ok {
		return nil, fmt.Errorf("failed to subscribe raydium cpmm swaps: pool=invalid")
	}
	logs, err := s.subscribe(poolAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe raydium cpmm swaps: %w", err)
	}
	return &SwapSubscription{source: s.source, logs: logs, pool: pool}, nil
}

// Recv waits for the next transaction that changes both configured pool vaults.
//
// Parameters:
//   - ctx: receive context.
//
// Returns:
//   - Normalized Raydium CPMM swap.
//   - Receive or decoding error.
//
// Version:
//   - 2026-09-01: Added.
func (s *SwapSubscription) Recv(ctx context.Context) (*SwapEvent, error) {
	if s == nil || s.logs == nil || s.source == nil {
		return nil, fmt.Errorf("failed to receive raydium cpmm swap: subscription=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		log, err := s.logs.Recv(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to receive raydium cpmm swap: %w", err)
		}
		if log == nil || log.Failed {
			continue
		}
		transaction, err := s.source.Transaction(ctx, log.Signature)
		if err != nil {
			return nil, fmt.Errorf("failed to receive raydium cpmm swap: %w", err)
		}
		if transaction.Failed {
			continue
		}
		swap, found, err := ammtransaction.DeriveSwap(transaction, s.pool.Token0Vault, s.pool.Token0Mint, s.pool.Token1Vault, s.pool.Token1Mint)
		if err != nil {
			return nil, fmt.Errorf("failed to receive raydium cpmm swap: %w", err)
		}
		if !found {
			continue
		}
		event := &SwapEvent{Signature: transaction.Signature, Slot: transaction.Slot, Pool: s.pool.Address, InputMint: swap.InputMint, OutputMint: swap.OutputMint, AmountIn: swap.AmountIn, AmountOut: swap.AmountOut}
		if transaction.Timestamp != nil {
			event.Timestamp = transaction.Timestamp.UTC()
		}
		return event, nil
	}
}

// Close closes the underlying Solana log subscription.
//
// Version:
//   - 2026-09-01: Added.
func (s *SwapSubscription) Close() {
	if s != nil && s.logs != nil {
		s.logs.Close()
	}
}
