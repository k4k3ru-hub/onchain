// Package health retrieves Arcus Spot Router health data.
package health

import (
	"context"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot/transport"
	"net/url"
)

type Params struct{}
type Result struct {
	OK      bool   `json:"ok"`
	ChainID uint64 `json:"chainId"`
}

type Client struct{ executor transport.Executor }

// NewClient composes an operation with its injected executor.
//
// Version:
//   - 2026-09-07: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create spot health client: executor=null")
	}
	return &Client{executor: e}, nil
}

// Send retrieves public Spot Router data without signing or submitting trades.
//
// Version:
//   - 2026-09-07: Added.
func (c *Client) Send(ctx context.Context, p Params) (*Result, error) {
	if c == nil || safety.IsNil(c.executor) {
		return nil, fmt.Errorf("failed to request spot health: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to request spot health: context=null")
	}
	q := url.Values{}
	var result Result
	if err := c.executor.Get(ctx, "/health", q, &result); err != nil {
		return nil, fmt.Errorf("failed to request spot health: %w", err)
	}
	return &result, nil
}
