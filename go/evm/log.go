// log.go
package evm

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

type logFilterer interface {
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
}

type logSubscriber interface {
	SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)
}

// FilterLogs filters EVM logs using the HTTP RPC client.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - query: EVM log filter query.
//
// Returns:
//   - Matching EVM logs.
//   - Filter error.
//
// Version:
//   - 2026-08-17: Added.
func (c *HTTPClient) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to filter evm logs: evm_http_client=null")
	}
	if c.logFilterer == nil {
		return nil, fmt.Errorf("failed to filter evm logs: http_eth_client=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logs, err := c.logFilterer.FilterLogs(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to filter evm logs: %w", err)
	}

	return logs, nil
}

// SubscribeFilterLogs subscribes to EVM logs using the WebSocket RPC client.
//
// Parameters:
//   - ctx: subscription context; nil uses context.Background.
//   - query: EVM log filter query.
//   - ch: destination for matching EVM logs.
//
// Returns:
//   - EVM log subscription managed by the caller.
//   - Subscription error.
//
// Version:
//   - 2026-08-17: Added.
func (c *WSClient) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to subscribe evm logs: evm_ws_client=null")
	}
	if c.logSubscriber == nil {
		return nil, fmt.Errorf("failed to subscribe evm logs: ws_eth_client=null")
	}
	if ch == nil {
		return nil, fmt.Errorf("failed to subscribe evm logs: logs_channel=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	subscription, err := c.logSubscriber.SubscribeFilterLogs(ctx, query, ch)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe evm logs: %w", err)
	}

	return subscription, nil
}
