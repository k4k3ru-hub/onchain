// Package markets composes Arcus public REST operations.
package markets

import (
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/bbo"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/candles"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/funding_rates"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/list"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/order_book"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/trades"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/transport"
)

type Client struct {
	List         *list.Client
	OrderBook    *order_book.Client
	BBO          *bbo.Client
	Trades       *trades.Client
	Candles      *candles.Client
	FundingRates *funding_rates.Client
}

// NewClient composes each public market operation around one executor.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(e transport.Executor) (*Client, error) {
	if safety.IsNil(e) {
		return nil, fmt.Errorf("failed to create markets client: executor=null")
	}
	c := &Client{}
	var err error
	c.List, err = list.NewClient(e)
	if err != nil {
		return nil, fmt.Errorf("failed to compose markets client: %w", err)
	}
	c.OrderBook, err = order_book.NewClient(e)
	if err != nil {
		return nil, fmt.Errorf("failed to compose markets client: %w", err)
	}
	c.BBO, err = bbo.NewClient(e)
	if err != nil {
		return nil, fmt.Errorf("failed to compose markets client: %w", err)
	}
	c.Trades, err = trades.NewClient(e)
	if err != nil {
		return nil, fmt.Errorf("failed to compose markets client: %w", err)
	}
	c.Candles, err = candles.NewClient(e)
	if err != nil {
		return nil, fmt.Errorf("failed to compose markets client: %w", err)
	}
	c.FundingRates, err = funding_rates.NewClient(e)
	if err != nil {
		return nil, fmt.Errorf("failed to compose markets client: %w", err)
	}
	return c, nil
}
