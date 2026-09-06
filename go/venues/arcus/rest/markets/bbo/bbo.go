// Package bbo implements the Arcus BBO operation.
package bbo

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/query"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/protocol"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/transport"
)

type Params struct{ Market string }
type Result struct{ protocol.BBO }
type Client struct{ executor transport.Executor }

// NewClient creates an operation with an explicitly injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create bbo client: executor=null")
	}
	return &Client{executor: e}, nil
}

// Send retrieves public market data using immutable request parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, p Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request bbo: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to request bbo: context=null")
	}
	q, err := query.RequiredMarket(p.Market)
	if err != nil {
		return nil, fmt.Errorf("failed to request bbo: %w", err)
	}
	q.Del("market")
	var result Result
	if err := c.executor.Get(ctx, "/v1/bbo/"+p.Market, q, &result); err != nil {
		return nil, fmt.Errorf("failed to request bbo: %w", err)
	}
	return &result, nil
}
