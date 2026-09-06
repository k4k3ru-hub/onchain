// Package recent_trades implements the Lighter recentTrades endpoint.
package recent_trades

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/query"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/protocol"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/transport"
)

type Client struct{ executor transport.Executor }
type Params struct {
	MarketID int64
	Limit    int64
}
type Result struct {
	Code       int              `json:"code"`
	NextCursor string           `json:"next_cursor"`
	Trades     []protocol.Trade `json:"trades"`
}

// NewClient creates an operation with an injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(executor transport.Executor) (*Client, error) {
	if safety.IsNil(executor) {
		return nil, fmt.Errorf("failed to create recent trades client: executor=null")
	}
	return &Client{executor: executor}, nil
}

// Send requests recent trades using immutable parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, params Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request recent trades: client=null")
	}
	values, err := (query.LimitParams{MarketID: params.MarketID, Limit: params.Limit, Maximum: 100}).Values()
	if err != nil {
		return nil, fmt.Errorf("failed to request recent trades: %w", err)
	}
	var result Result
	if err := c.executor.Get(ctx, "/api/v1/recentTrades", values, &result); err != nil {
		return nil, fmt.Errorf("failed to request recent trades: %w", err)
	}
	return &result, nil
}
