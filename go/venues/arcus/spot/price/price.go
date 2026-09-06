// Package price retrieves Arcus Spot Router price data.
package price

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot/transport"
	"math/big"
	"net/url"
	"strconv"
	"strings"
)

type Params struct {
	ChainID    uint64
	SellToken  string
	BuyToken   string
	SellAmount string
}
type Price struct {
	Venue      string          `json:"venue"`
	BuyAmount  string          `json:"buyAmount"`
	SellAmount string          `json:"sellAmount"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}
type VenueError struct {
	Venue string `json:"venue"`
	Error struct {
		Kind    string `json:"kind"`
		Status  int    `json:"status,omitempty"`
		Message string `json:"message"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}
type Result struct {
	Recommended string       `json:"recommended"`
	All         []Price      `json:"all"`
	Errors      []VenueError `json:"errors,omitempty"`
}

type Client struct{ executor transport.Executor }

// NewClient composes an operation with its injected executor.
//
// Version:
//   - 2026-09-07: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create spot price client: executor=null")
	}
	return &Client{executor: e}, nil
}

// Send retrieves public Spot Router data without signing or submitting trades.
//
// Version:
//   - 2026-09-07: Added.
func (c *Client) Send(ctx context.Context, p Params) (*Result, error) {
	if c == nil || safety.IsNil(c.executor) {
		return nil, fmt.Errorf("failed to request spot price: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to request spot price: context=null")
	}
	if !common.IsHexAddress(p.SellToken) || !common.IsHexAddress(p.BuyToken) {
		return nil, fmt.Errorf("failed to request spot price: token_address=invalid")
	}
	if strings.EqualFold(p.SellToken, p.BuyToken) {
		return nil, fmt.Errorf("failed to request spot price: token_pair=invalid")
	}
	if len(p.SellAmount) == 0 {
		return nil, fmt.Errorf("failed to request spot price: sell_amount=empty")
	}
	if len(p.SellAmount) > 78 {
		return nil, fmt.Errorf("failed to request spot price: sell_amount=too_long max_length=78")
	}
	for _, digit := range p.SellAmount {
		if digit < '0' || digit > '9' {
			return nil, fmt.Errorf("failed to request spot price: sell_amount=invalid")
		}
	}
	amount, ok := new(big.Int).SetString(p.SellAmount, 10)
	if !ok || amount.Sign() <= 0 || amount.BitLen() > 256 {
		return nil, fmt.Errorf("failed to request spot price: sell_amount=out_of_range")
	}
	q := url.Values{"sellToken": {p.SellToken}, "buyToken": {p.BuyToken}, "sellAmount": {p.SellAmount}}
	if p.ChainID != 0 {
		q.Set("chainId", strconv.FormatUint(p.ChainID, 10))
	}

	var result Result
	if err := c.executor.Get(ctx, "/v1/price", q, &result); err != nil {
		return nil, fmt.Errorf("failed to request spot price: %w", err)
	}
	return &result, nil
}
