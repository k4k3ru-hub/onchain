// Package candles implements the Arcus Candles operation.
package candles

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
	Timeframe string
	To        int64
	From      *int64
	CountBack int64
}
type Result struct {
	Candles []protocol.Candle `json:"candles"`
}
type Client struct{ executor transport.Executor }

// NewClient creates an operation with an explicitly injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create candles client: executor=null")
	}
	return &Client{executor: e}, nil
}

// Send retrieves public market data using immutable request parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, p Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request candles: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to request candles: context=null")
	}
	q, err := query.Candles(p.Market, p.Timeframe, p.To, p.From, p.CountBack)
	if err != nil {
		return nil, fmt.Errorf("failed to request candles: %w", err)
	}

	var result Result
	if err := c.executor.Get(ctx, "/v1/candles", q, &result); err != nil {
		return nil, fmt.Errorf("failed to request candles: %w", err)
	}
	return &result, nil
}
