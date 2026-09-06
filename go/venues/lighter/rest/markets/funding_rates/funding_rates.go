// Package funding_rates implements the Lighter funding-rates endpoint.
package funding_rates

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/query"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/protocol"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/transport"
)

type Client struct{ executor transport.Executor }
type Params struct{}
type Result struct {
	Code         int                    `json:"code"`
	FundingRates []protocol.FundingRate `json:"funding_rates"`
}

// NewClient creates an operation with an injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(executor transport.Executor) (*Client, error) {
	if safety.IsNil(executor) {
		return nil, fmt.Errorf("failed to create funding rates client: executor=null")
	}
	return &Client{executor: executor}, nil
}

// Send requests funding rates using immutable parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, params Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request funding rates: client=null")
	}
	values, err := (query.EmptyParams{}).Values()
	if err != nil {
		return nil, fmt.Errorf("failed to request funding rates: %w", err)
	}
	var result Result
	if err := c.executor.Get(ctx, "/api/v1/funding-rates", values, &result); err != nil {
		return nil, fmt.Errorf("failed to request funding rates: %w", err)
	}
	return &result, nil
}
