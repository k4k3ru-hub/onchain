package clmm

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
	Signature  onchainSolana.Signature
	Slot       onchainSolana.Slot
	Timestamp  time.Time
	Pool       onchainSolana.Address
	InputMint  onchainSolana.Address
	OutputMint onchainSolana.Address
	AmountIn   uint64
	AmountOut  uint64
	EventIndex uint32
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

// NewSwapSubscriber creates a Raydium CLMM swap subscriber for discovered pools.
//
// Parameters:
//   - client: Raydium CLMM client containing discovered pools.
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
		return nil, fmt.Errorf("failed to create raydium clmm swap subscriber: dependency=null")
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
		return nil, fmt.Errorf("failed to subscribe raydium clmm swaps: subscriber=null")
	}
	pool, ok := s.pools[poolAddress]
	if !ok {
		return nil, fmt.Errorf("failed to subscribe raydium clmm swaps: pool=invalid")
	}
	logs, err := s.subscribe(poolAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe raydium clmm swaps: %w", err)
	}
	return &SwapSubscription{source: s.source, logs: logs, pool: pool}, nil
}

// Swap resolves one transaction signature into a configured Raydium CLMM swap.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - poolAddress: configured pool address.
//   - signature: transaction signature to inspect.
//
// Returns:
//   - Normalized swap, or nil when the transaction is failed or does not change both pool vaults.
//   - Resolution error.
//
// Version:
//   - 2026-09-01: Added.
func (s *SwapSubscriber) Swap(ctx context.Context, poolAddress onchainSolana.Address, signature onchainSolana.Signature) (*SwapEvent, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("failed to resolve raydium clmm swap: subscriber=null")
	}
	pool, ok := s.pools[poolAddress]
	if !ok {
		return nil, fmt.Errorf("failed to resolve raydium clmm swap: pool=invalid")
	}
	if signature.IsZero() {
		return nil, fmt.Errorf("failed to resolve raydium clmm swap: signature=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return resolveSwap(ctx, s.source, pool, signature)
}

// Recv waits for the next transaction that changes both configured pool vaults.
//
// Parameters:
//   - ctx: receive context.
//
// Returns:
//   - Normalized Raydium CLMM swap.
//   - Receive or decoding error.
//
// Version:
//   - 2026-09-01: Added.
func (s *SwapSubscription) Recv(ctx context.Context) (*SwapEvent, error) {
	if s == nil || s.logs == nil || s.source == nil {
		return nil, fmt.Errorf("failed to receive raydium clmm swap: subscription=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		log, err := s.logs.Recv(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to receive raydium clmm swap: %w", err)
		}
		if log == nil || log.Failed {
			continue
		}
		event, err := resolveSwap(ctx, s.source, s.pool, log.Signature)
		if err != nil {
			return nil, fmt.Errorf("failed to receive raydium clmm swap: %w", err)
		}
		if event != nil {
			return event, nil
		}
	}
}

func resolveSwap(ctx context.Context, source transactionSource, pool Pool, signature onchainSolana.Signature) (*SwapEvent, error) {
	transaction, err := source.Transaction(ctx, signature)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve raydium clmm swap: %w", err)
	}
	if transaction == nil {
		return nil, fmt.Errorf("failed to resolve raydium clmm swap: transaction=null")
	}
	if transaction.Failed {
		return nil, nil
	}
	swap, found, err := ammtransaction.DeriveSwap(transaction, pool.Token0Vault, pool.Token0Mint, pool.Token1Vault, pool.Token1Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve raydium clmm swap: %w", err)
	}
	if !found {
		return nil, nil
	}
	event := &SwapEvent{Signature: transaction.Signature, Slot: transaction.Slot, Pool: pool.Address, InputMint: swap.InputMint, OutputMint: swap.OutputMint, AmountIn: swap.AmountIn, AmountOut: swap.AmountOut}
	if transaction.Timestamp != nil {
		event.Timestamp = transaction.Timestamp.UTC()
	}
	return event, nil
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
