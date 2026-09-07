package slipstream

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

const swapLogBufferSize = 64

type WSRPCClient interface {
	SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)
}

type SwapSubscriberParams struct {
	RPC     WSRPCClient
	Sources []SwapSource
}

type SwapSubscriber struct {
	rpc         WSRPCClient
	poolKeys    map[common.Address]protocol.PoolKey
	poolAddress []common.Address
}

type SwapSubscription struct {
	swaps        chan Swap
	errs         chan error
	cancel       context.CancelFunc
	subscription ethereum.Subscription
	stopOnce     sync.Once
}

// NewSwapSubscriber creates a configured live Swap subscriber.
//
// Parameters:
//   - params: WebSocket RPC dependency and pool sources.
//
// Returns:
//   - Live Swap subscriber.
//   - Client creation error.
//
// Version:
//   - 2026-08-30: Added.
func NewSwapSubscriber(params SwapSubscriberParams) (*SwapSubscriber, error) {
	if params.RPC == nil {
		return nil, fmt.Errorf("failed to create slipstream swap subscriber: ws_rpc_client=null")
	}
	poolKeys, poolAddresses, err := buildSwapSources(params.Sources)
	if err != nil {
		return nil, fmt.Errorf("failed to create slipstream swap subscriber: %w", err)
	}
	return &SwapSubscriber{rpc: params.RPC, poolKeys: poolKeys, poolAddress: poolAddresses}, nil
}

// Swaps returns the decoded Swap event channel.
//
// Returns:
//   - Swap event channel, closed when the subscription stops.
//
// Version:
//   - 2026-08-30: Added.
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
//   - 2026-08-30: Added.
func (s *SwapSubscription) Err() <-chan error {
	if s == nil {
		return nil
	}
	return s.errs
}

// Unsubscribe stops the Swap subscription.
//
// Version:
//   - 2026-08-30: Added.
func (s *SwapSubscription) Unsubscribe() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.cancel()
		s.subscription.Unsubscribe()
	})
}

// SubscribeSwaps subscribes to Swap events for configured Slipstream pools.
//
// Parameters:
//   - ctx: Subscription context; nil uses context.Background.
//
// Returns:
//   - Managed Swap subscription.
//   - Subscription creation error.
//
// Version:
//   - 2026-08-30: Added.
func (c *SwapSubscriber) SubscribeSwaps(ctx context.Context) (*SwapSubscription, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to subscribe slipstream swaps: swap_subscriber=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	subscriptionCtx, cancel := context.WithCancel(ctx)
	logs := make(chan types.Log, swapLogBufferSize)
	source, err := c.rpc.SubscribeFilterLogs(subscriptionCtx, ethereum.FilterQuery{
		Addresses: append([]common.Address(nil), c.poolAddress...),
		Topics:    [][]common.Hash{{swapEventSignatureHash()}},
	}, logs)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe slipstream swaps: %w", err)
	}
	if source == nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe slipstream swaps: subscription=null")
	}
	subscription := &SwapSubscription{
		swaps:        make(chan Swap, swapLogBufferSize),
		errs:         make(chan error, 1),
		cancel:       cancel,
		subscription: source,
	}
	go c.consumeSwapLogs(subscriptionCtx, logs, subscription)
	return subscription, nil
}

func (c *SwapSubscriber) consumeSwapLogs(ctx context.Context, logs <-chan types.Log, subscription *SwapSubscription) {
	defer close(subscription.swaps)
	defer close(subscription.errs)
	defer subscription.Unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-subscription.subscription.Err():
			if ok && err != nil {
				subscription.errs <- fmt.Errorf("failed to consume slipstream swap subscription: %w", err)
			}
			return
		case eventLog, ok := <-logs:
			if !ok {
				subscription.errs <- fmt.Errorf("failed to consume slipstream swap subscription: logs_channel=closed")
				return
			}
			poolKey, exists := c.poolKeys[eventLog.Address]
			if !exists {
				subscription.errs <- fmt.Errorf("failed to consume slipstream swap subscription: pool_address=unconfigured")
				return
			}
			swap, err := DecodeSwapLog(eventLog)
			if err != nil {
				subscription.errs <- fmt.Errorf("failed to consume slipstream swap subscription: %w", err)
				return
			}
			swap.PoolKey = poolKey
			select {
			case subscription.swaps <- swap:
			case <-ctx.Done():
				return
			}
		}
	}
}
