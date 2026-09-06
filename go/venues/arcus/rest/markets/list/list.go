// Package list implements the Arcus List operation.
package list

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/query"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/protocol"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/transport"
)

type Params struct{ Market string }
type Result struct {
	Markets []protocol.MarketInfo `json:"markets"`
}
type Client struct{ executor transport.Executor }

// NewClient creates an operation with an explicitly injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create list client: executor=null")
	}
	return &Client{executor: e}, nil
}

// Send retrieves public market data using immutable request parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, p Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request list: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to request list: context=null")
	}
	q, err := query.OptionalMarket(p.Market)
	if err != nil {
		return nil, fmt.Errorf("failed to request list: %w", err)
	}

	var result Result
	if err := c.executor.Get(ctx, "/v1/markets", q, &result); err != nil {
		return nil, fmt.Errorf("failed to request list: %w", err)
	}
	return &result, nil
}
