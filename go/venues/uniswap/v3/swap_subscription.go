package v3

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const swapLogBufferSize = 64

type SwapSubscription struct {
	swaps        chan Swap
	errs         chan error
	cancel       context.CancelFunc
	subscription ethereum.Subscription
	stopOnce     sync.Once
}

// Swaps returns the decoded Swap event channel.
//
// Returns:
//   - Swap event channel, closed when the subscription stops.
//
// Version:
//   - 2026-08-24: Added.
func (s *SwapSubscription) Swaps() <-chan Swap {
	if s == nil {
		return nil
	}
	return s.swaps
}

// Err returns the terminal subscription error channel.
//
// Returns:
//   - Buffered error channel, closed when the subscription stops.
//
// Version:
//   - 2026-08-24: Added.
func (s *SwapSubscription) Err() <-chan error {
	if s == nil {
		return nil
	}
	return s.errs
}

// Unsubscribe stops the Swap subscription.
//
// Version:
//   - 2026-08-24: Added.
func (s *SwapSubscription) Unsubscribe() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { s.cancel(); s.subscription.Unsubscribe() })
}

// SubscribeSwaps subscribes to Swap events for the configured Uniswap v3 pools.
//
// Parameters:
//   - ctx: subscription context; nil uses context.Background.
//
// Returns:
//   - Managed Swap subscription.
//   - Subscription creation error.
//
// Version:
//   - 2026-08-24: Added.
func (c *WSClient) SubscribeSwaps(ctx context.Context) (*SwapSubscription, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to subscribe uniswap v3 swaps: client=null")
	}
	if c.rpc == nil {
		return nil, fmt.Errorf("failed to subscribe uniswap v3 swaps: ws_rpc_client=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	subscriptionCtx, cancel := context.WithCancel(ctx)
	logs := make(chan types.Log, swapLogBufferSize)
	source, err := c.rpc.SubscribeFilterLogs(subscriptionCtx, ethereum.FilterQuery{Addresses: poolAddresses(c.poolByAdd), Topics: [][]common.Hash{{swapEventSignatureHash()}}}, logs)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe uniswap v3 swaps: %w", err)
	}
	if source == nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe uniswap v3 swaps: subscription=null")
	}
	s := &SwapSubscription{swaps: make(chan Swap, swapLogBufferSize), errs: make(chan error, 1), cancel: cancel, subscription: source}
	go c.consumeSwapLogs(subscriptionCtx, logs, s)
	return s, nil
}

func (c *WSClient) consumeSwapLogs(ctx context.Context, logs <-chan types.Log, s *SwapSubscription) {
	defer close(s.swaps)
	defer close(s.errs)
	defer s.Unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-s.subscription.Err():
			if ok && err != nil {
				s.errs <- fmt.Errorf("failed to consume uniswap v3 swap subscription: %w", err)
			}
			return
		case eventLog, ok := <-logs:
			if !ok {
				s.errs <- fmt.Errorf("failed to consume uniswap v3 swap subscription: logs_channel=closed")
				return
			}
			poolKey, configured := configuredPool(c.poolByAdd, eventLog.Address)
			if !configured {
				s.errs <- fmt.Errorf("failed to consume uniswap v3 swap subscription: pool_address=unconfigured")
				return
			}
			swap, err := DecodeSwapLog(eventLog)
			if err != nil {
				s.errs <- fmt.Errorf("failed to consume uniswap v3 swap subscription: %w", err)
				return
			}
			swap.PoolKey = poolKey
			select {
			case s.swaps <- swap:
			case <-ctx.Done():
				return
			}
		}
	}
}
