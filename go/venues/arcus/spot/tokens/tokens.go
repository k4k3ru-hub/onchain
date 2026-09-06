// Package tokens retrieves Arcus Spot Router tokens data.
package tokens

import (
	"context"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot/transport"
	"net/url"
)

type Params struct{}
type Token struct {
	ChainID             uint64  `json:"chainId"`
	Address             string  `json:"address"`
	Symbol              string  `json:"symbol"`
	Name                string  `json:"name"`
	Decimals            int     `json:"decimals"`
	Source              string  `json:"source"`
	Category            string  `json:"category"`
	Verified            bool    `json:"verified"`
	WrappedTokenAddress *string `json:"wrappedTokenAddress,omitempty"`
}
type Result []Token

type Client struct{ executor transport.Executor }

// NewClient composes an operation with its injected executor.
//
// Version:
//   - 2026-09-07: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create spot tokens client: executor=null")
	}
	return &Client{executor: e}, nil
}

// Send retrieves public Spot Router data without signing or submitting trades.
//
// Version:
//   - 2026-09-07: Added.
func (c *Client) Send(ctx context.Context, p Params) (*Result, error) {
	if c == nil || safety.IsNil(c.executor) {
		return nil, fmt.Errorf("failed to request spot tokens: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to request spot tokens: context=null")
	}
	q := url.Values{}
	var result Result
	if err := c.executor.Get(ctx, "/v1/tokens", q, &result); err != nil {
		return nil, fmt.Errorf("failed to request spot tokens: %w", err)
	}
	return &result, nil
}
