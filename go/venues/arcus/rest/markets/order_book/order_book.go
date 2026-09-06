// Package order_book implements the Arcus OrderBook operation.
package order_book

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/query"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/protocol"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/transport"
)

type Params struct {
	Market    string
	NLevels   int64
	SigFigs   int64
	RoundStep int64
}
type Result struct{ protocol.OrderbookSnapshot }
type Client struct{ executor transport.Executor }

// NewClient creates an operation with an explicitly injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create order book client: executor=null")
	}
	return &Client{executor: e}, nil
}

// Send retrieves public market data using immutable request parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, p Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request order book: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to request order book: context=null")
	}
	q, err := query.Book(p.Market, p.NLevels, p.SigFigs, p.RoundStep)
	if err != nil {
		return nil, fmt.Errorf("failed to request order book: %w", err)
	}
	q.Del("market")
	var result Result
	if err := c.executor.Get(ctx, "/v1/l2OrderBook/"+p.Market, q, &result); err != nil {
		return nil, fmt.Errorf("failed to request order book: %w", err)
	}
	return &result, nil
}
