// Package fundings implements the Lighter fundings endpoint.
package fundings

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
	MarketID       int64
	Resolution     string
	StartTimestamp int64
	EndTimestamp   int64
	CountBack      int64
}
type Result struct {
	Code       int                `json:"code"`
	Resolution string             `json:"resolution"`
	Fundings   []protocol.Funding `json:"fundings"`
}

// NewClient creates an operation with an injected executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(executor transport.Executor) (*Client, error) {
	if safety.IsNil(executor) {
		return nil, fmt.Errorf("failed to create fundings client: executor=null")
	}
	return &Client{executor: executor}, nil
}

// Send requests fundings using immutable parameters.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, params Params) (*Result, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to request fundings: client=null")
	}
	values, err := (query.FundingParams{MarketID: params.MarketID, Resolution: params.Resolution, StartTimestamp: params.StartTimestamp, EndTimestamp: params.EndTimestamp, CountBack: params.CountBack}).Values()
	if err != nil {
		return nil, fmt.Errorf("failed to request fundings: %w", err)
	}
	var result Result
	if err := c.executor.Get(ctx, "/api/v1/fundings", values, &result); err != nil {
		return nil, fmt.Errorf("failed to request fundings: %w", err)
	}
	return &result, nil
}
