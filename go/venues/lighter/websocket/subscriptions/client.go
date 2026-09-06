// Package subscriptions owns public Lighter subscription operations.
package subscriptions

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/query"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket/protocol"
)

type Executor interface {
	Send(context.Context, []byte) error
}
type Params struct {
	MarketID int64
	All      bool
}
type Client struct {
	executor Executor
	channel  string
	allowAll bool
}

// NewClient creates one public subscription group using an injected sender.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(executor Executor, channel string) (*Client, error) {
	if safety.IsNil(executor) {
		return nil, fmt.Errorf("failed to create subscription client: executor=null")
	}
	all := false
	switch channel {
	case "order_book", "ticker", "trade":
	case "market_stats", "spot_market_stats":
		all = true
	default:
		return nil, fmt.Errorf("failed to create subscription client: channel=invalid")
	}
	return &Client{executor: executor, channel: channel, allowAll: all}, nil
}

// Subscribe sends a subscription request; success is a send acknowledgement only.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Subscribe(ctx context.Context, p Params) error {
	return c.execute(ctx, "subscribe", p)
}

// Unsubscribe sends an unsubscribe request for the same channel and parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Unsubscribe(ctx context.Context, p Params) error {
	return c.execute(ctx, "unsubscribe", p)
}
func (c *Client) execute(ctx context.Context, kind string, p Params) error {
	if c == nil {
		return fmt.Errorf("failed to %s lighter channel: client=null", kind)
	}
	if err := query.MarketID(p.MarketID); err != nil {
		return fmt.Errorf("failed to %s lighter channel: %w", kind, err)
	}
	key := strconv.FormatInt(p.MarketID, 10)
	if p.All {
		if !c.allowAll || p.MarketID != 0 {
			return fmt.Errorf("failed to %s lighter channel: all_market_selection=invalid", kind)
		}
		key = "all"
	}
	payload, err := json.Marshal(protocol.Request{Type: kind, Channel: c.channel + "/" + key})
	if err != nil {
		return fmt.Errorf("failed to encode subscription: %w", err)
	}
	if err := c.executor.Send(ctx, payload); err != nil {
		return fmt.Errorf("failed to %s lighter channel: %w", kind, err)
	}
	return nil
}
