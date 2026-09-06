// Package order_books implements the Lighter orderBooks endpoint.
package order_books

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
	MarketID *int64
	Filter   string
}
type Result struct {
	Code       int                  `json:"code"`
	OrderBooks []protocol.OrderBook `json:"order_books"`
}

// NewClient creates an operation with an injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(executor transport.Executor) (*Client, error) {
	if safety.IsNil(executor) {
		return nil, fmt.Errorf("failed to create order books client: executor=null")
	}
	return &Client{executor: executor}, nil
}

// Send requests order books using immutable parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, params Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request order books: client=null")
	}
	values, err := (query.MetadataParams{MarketID: params.MarketID, Filter: params.Filter}).Values()
	if err != nil {
		return nil, fmt.Errorf("failed to request order books: %w", err)
	}
	var result Result
	if err := c.executor.Get(ctx, "/api/v1/orderBooks", values, &result); err != nil {
		return nil, fmt.Errorf("failed to request order books: %w", err)
	}
	return &result, nil
}
