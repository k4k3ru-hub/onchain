// Package order_book_details implements the Lighter orderBookDetails endpoint.
package order_book_details

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
	Code                 int                             `json:"code"`
	OrderBookDetails     []protocol.PerpsOrderBookDetail `json:"order_book_details"`
	SpotOrderBookDetails []protocol.SpotOrderBookDetail  `json:"spot_order_book_details"`
}

// NewClient creates an operation with an injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(executor transport.Executor) (*Client, error) {
	if safety.IsNil(executor) {
		return nil, fmt.Errorf("failed to create order book details client: executor=null")
	}
	return &Client{executor: executor}, nil
}

// Send requests order book details using immutable parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, params Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request order book details: client=null")
	}
	values, err := (query.MetadataParams{MarketID: params.MarketID, Filter: params.Filter}).Values()
	if err != nil {
		return nil, fmt.Errorf("failed to request order book details: %w", err)
	}
	var result Result
	if err := c.executor.Get(ctx, "/api/v1/orderBookDetails", values, &result); err != nil {
		return nil, fmt.Errorf("failed to request order book details: %w", err)
	}
	return &result, nil
}
