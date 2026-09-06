// Package subscriptions owns Arcus public channel operations.
package subscriptions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/query"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
)

type Executor interface {
	Send(context.Context, []byte) error
}
type Params struct {
	Market    string
	NLevels   int64
	SigFigs   int64
	RoundStep int64
	Snapshot  *bool
}
type Client struct {
	executor Executor
	channel  string
}

// NewClient composes a documented public channel around an explicit sender.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(e Executor, channel string) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create subscription client: executor=null")
	}
	switch channel {
	case "l2Orderbook", "l2OrderbookUpdates", "bbo", "trades", "markets", "oraclePrices", "predictedFunding":
	default:
		return nil, fmt.Errorf("failed to create subscription client: channel=invalid")
	}
	return &Client{executor: e, channel: channel}, nil
}

// Subscribe sends a request; Recv delivers server acceptance and initial data.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Subscribe(ctx context.Context, p Params) error {
	return c.execute(ctx, "subscribe", p)
}

// Unsubscribe sends the market and aggregation identity used to subscribe.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Unsubscribe(ctx context.Context, p Params) error {
	return c.execute(ctx, "unsubscribe", p)
}
func (c *Client) execute(ctx context.Context, kind string, p Params) error {
	if c == nil {
		return fmt.Errorf("failed to send subscription: client=null")
	}
	if ctx == nil {
		return fmt.Errorf("failed to send subscription: context=null")
	}
	book := c.channel == "l2Orderbook" || c.channel == "l2OrderbookUpdates"
	if c.channel == "markets" {
		if p.Market != "" {
			return fmt.Errorf("failed to validate subscription: market=invalid")
		}
	} else if _, err := query.RequiredMarket(p.Market); err != nil {
		return fmt.Errorf("failed to validate subscription: %w", err)
	}
	if book {
		if _, err := query.Book(p.Market, p.NLevels, p.SigFigs, p.RoundStep); err != nil {
			return fmt.Errorf("failed to validate subscription: %w", err)
		}
	} else if p.NLevels != 0 || p.SigFigs != 0 || p.RoundStep != 0 {
		return fmt.Errorf("failed to validate subscription: book_options=invalid")
	}
	payload := struct {
		Type      string `json:"type"`
		Channel   string `json:"channel"`
		ID        string `json:"id,omitempty"`
		NLevels   int64  `json:"nLevels,omitempty"`
		SigFigs   int64  `json:"sigFigs,omitempty"`
		RoundStep int64  `json:"roundStep,omitempty"`
		Snapshot  *bool  `json:"snapshot,omitempty"`
	}{Type: kind, Channel: c.channel, ID: p.Market, SigFigs: p.SigFigs, RoundStep: p.RoundStep}
	if kind == "subscribe" {
		payload.NLevels = p.NLevels
		payload.Snapshot = p.Snapshot
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode subscription: %w", err)
	}
	if err := c.executor.Send(ctx, b); err != nil {
		return fmt.Errorf("failed to %s arcus channel: %w", kind, err)
	}
	return nil
}
