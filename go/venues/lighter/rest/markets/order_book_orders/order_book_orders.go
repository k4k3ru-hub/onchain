// Package order_book_orders implements the Lighter orderBookOrders endpoint.
package order_book_orders

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
	Code      int                    `json:"code"`
	TotalAsks int64                  `json:"total_asks"`
	Asks      []protocol.SimpleOrder `json:"asks"`
	TotalBids int64                  `json:"total_bids"`
	Bids      []protocol.SimpleOrder `json:"bids"`
}

// NewClient creates an operation with an injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(executor transport.Executor) (*Client, error) {
	if safety.IsNil(executor) {
		return nil, fmt.Errorf("failed to create order book orders client: executor=null")
	}
	return &Client{executor: executor}, nil
}

// Send requests order book orders using immutable parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, params Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request order book orders: client=null")
	}
	values, err := (query.LimitParams{MarketID: params.MarketID, Limit: params.Limit, Maximum: 250}).Values()
	if err != nil {
		return nil, fmt.Errorf("failed to request order book orders: %w", err)
	}
	var result Result
	if err := c.executor.Get(ctx, "/api/v1/orderBookOrders", values, &result); err != nil {
		return nil, fmt.Errorf("failed to request order book orders: %w", err)
	}
	return &result, nil
}
