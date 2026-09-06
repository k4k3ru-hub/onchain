// Package funding_rates implements the Arcus FundingRates operation.
package funding_rates

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/query"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/protocol"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/transport"
)

type Params struct {
	Market string
	Limit  int64
	From   *int64
	To     *int64
}
type Result struct {
	FundingRates []protocol.MarketFundingRate `json:"fundingRates"`
}
type Client struct{ executor transport.Executor }

// NewClient creates an operation with an explicitly injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create funding rates client: executor=null")
	}
	return &Client{executor: e}, nil
}

// Send retrieves public market data using immutable request parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, p Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request funding rates: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to request funding rates: context=null")
	}
	q, err := query.History(p.Market, p.Limit, p.From, p.To)
	if err != nil {
		return nil, fmt.Errorf("failed to request funding rates: %w", err)
	}

	var result Result
	if err := c.executor.Get(ctx, "/v1/fundingRates", q, &result); err != nil {
		return nil, fmt.Errorf("failed to request funding rates: %w", err)
	}
	return &result, nil
}
